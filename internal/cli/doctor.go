package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/UrbanCompass/hnvm/internal/output"
	"github.com/UrbanCompass/hnvm/internal/resolve"
	"github.com/UrbanCompass/hnvm/internal/shim"
)

// DoctorCmd checks the installation.
type DoctorCmd struct {
	JSON    bool `kong:"help='Print the results as JSON.'"`
	Offline bool `kong:"help='Skip the network reachability checks.'"`
}

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, warn, fail
	Detail string `json:"detail"`
}

func (c DoctorCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	var checks []check
	add := func(name, status, format string, args ...any) {
		checks = append(checks, check{name, status, fmt.Sprintf(format, args...)})
	}

	// Shims first on PATH: anything else named node (nvm, volta, brew) silently bypasses hnvm.
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	for _, tool := range shim.Tools {
		p, err := exec.LookPath(tool)
		if err != nil {
			add("path:"+tool, "warn", "%s is not on PATH (run `hnvm shims <dir>` and add the directory to PATH)", tool)
			continue
		}
		if isHnvm(p, self) {
			add("path:"+tool, "ok", "%s", p)
		} else {
			add("path:"+tool, "fail", "%s is first on PATH and is not hnvm, so it shadows the hnvm shim", p)
		}
	}

	root := s.Root()
	if err := os.MkdirAll(root, 0o755); err != nil {
		add("hnvm_path", "fail", "cannot create %s: %v", root, err)
	} else if f, err := os.CreateTemp(root, ".doctor-*"); err != nil {
		add("hnvm_path", "fail", "%s is not writable: %v", root, err)
	} else {
		f.Close()
		os.Remove(f.Name())
		add("hnvm_path", "ok", "%s is writable", root)
	}

	// Leftovers of interrupted installs. Go installs extract under .tmp; bash extracted in place.
	if entries, err := os.ReadDir(filepath.Join(root, ".tmp")); err == nil {
		var stale []string
		for _, e := range entries {
			if info, err := e.Info(); err == nil && e.IsDir() && time.Since(info.ModTime()) > time.Hour && strings.Contains(e.Name(), "-") {
				stale = append(stale, e.Name())
			}
		}
		if len(stale) > 0 {
			add("stale_tmp", "warn", "interrupted installs in %s/.tmp: %s (safe to delete)", root, strings.Join(stale, ", "))
		}
	}
	for _, tool := range managedTools {
		for _, v := range resolve.Installed(root, tool) {
			if !installed(root, tool, v) {
				add("install:"+tool+"@"+v, "warn", "%s/%s/%s is incomplete; run `hnvm uninstall %s@%s`", root, tool, v, tool, v)
			}
		}
	}
	if fi, err := os.Stat(filepath.Join(root, "node")); err == nil && fi.IsDir() {
		for _, v := range resolve.Installed(root, "node") {
			lock := filepath.Join(root, "node", v, ".hnvm-distrust-lock")
			if fi, err := os.Stat(lock); err == nil && time.Since(fi.ModTime()) > 2*time.Minute {
				add("distrust_lock:"+v, "warn", "stale lock %s (removed automatically on the next run)", lock)
			}
		}
	}

	// Distrusted certificates in each installed node.
	if s.Cfg.Get("HNVM_DISTRUST_CERTS") != "false" {
		for _, v := range resolve.Installed(root, "node") {
			marker, err := os.ReadFile(filepath.Join(root, "node", v, ".hnvm-distrusted-certs"))
			switch {
			case err != nil:
				add("distrust:node@"+v, "warn", "not processed yet (happens on the next run of this version)")
			case strings.Contains(string(marker), "\ninvalid="):
				add("distrust:node@"+v, "warn", "processed, but HNVM_DISTRUST_CERTS_EXTRA had invalid fingerprints")
			default:
				add("distrust:node@"+v, "ok", "distrusted roots removed")
			}
		}
	}

	// The node version configured here, and whether min-safe-node raises it.
	ctx := context.Background()
	if spec := s.Cfg.Get("HNVM_NODE"); spec != "" {
		res, err := s.Res.Quiet(ctx, "node", spec)
		if err != nil {
			add("node_version", "fail", "cannot resolve HNVM_NODE=%q: %s", spec, errText(err))
		} else if raised := minSafe(s, res.Version); raised != res.Version {
			add("node_version", "warn", "pinned %s has known vulnerabilities; hnvm runs %s instead (pin it to silence this)", res.Version, raised)
		} else {
			add("node_version", "ok", "%s", res.Version)
		}
	} else {
		add("node_version", "warn", "no HNVM_NODE configured for %s", mustGetwd())
	}

	if keys := s.Managed.Keys(); len(keys) > 0 {
		var parts []string
		for _, k := range keys {
			v, _ := s.Managed.Raw(k)
			parts = append(parts, k+"="+v)
		}
		add("managed_profile", "ok", "%s", strings.Join(parts, ", "))
	}

	if !c.Offline {
		client := &http.Client{Timeout: 10 * time.Second}
		for _, key := range []string{"HNVM_NODE_DIST", "HNVM_PNPM_REGISTRY"} {
			url := s.Cfg.Get(key)
			req, _ := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
			if req == nil {
				add("reach:"+key, "fail", "invalid URL %q", url)
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				add("reach:"+key, "fail", "%s: %v", url, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				add("reach:"+key, "fail", "%s: %s", url, resp.Status)
			} else {
				add("reach:"+key, "ok", "%s", url)
			}
		}
	}

	failed := false
	for _, ch := range checks {
		failed = failed || ch.Status == "fail"
	}
	if c.JSON {
		enc := json.NewEncoder(g.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(checks); err != nil {
			return err
		}
	} else {
		marks := map[string]string{"ok": "✓", "warn": "!", "fail": "✗"}
		for _, ch := range checks {
			fmt.Fprintf(g.Stdout, "%s %s: %s\n", marks[ch.Status], ch.Name, ch.Detail)
		}
	}
	if failed {
		return exitCode(1)
	}
	return nil
}

// isHnvm reports whether p is this binary or one of the bash bootstrap shims that run it.
func isHnvm(p, self string) bool {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return false
	}
	if resolved == self {
		return true
	}
	data, err := os.ReadFile(resolved)
	if err != nil || len(data) > 64*1024 {
		return false
	}
	return strings.Contains(string(data), "hnvm_bootstrap_exec")
}

func mustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}

// quietOut silences informational output (but not errors) for commands that print their own.
func quietOut() *output.Output {
	return output.NewWriter(nil, true)
}
