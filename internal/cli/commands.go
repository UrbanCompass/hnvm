package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/UrbanCompass/hnvm/internal/install"
	"github.com/UrbanCompass/hnvm/internal/resolve"
	"github.com/UrbanCompass/hnvm/internal/semver"
	"github.com/UrbanCompass/hnvm/internal/shim"
)

// managedTools are the tools with their own version setting and install directory.
var managedTools = []string{"node", "pnpm", "yarn", "bun"}

func versionKey(tool string) string { return "HNVM_" + strings.ToUpper(tool) }

// VersionCmd prints the hnvm version.
type VersionCmd struct{}

func (VersionCmd) Run(g *Globals) error {
	fmt.Fprintln(g.Stdout, g.Version)
	return nil
}

// CurrentCmd shows what each tool resolves to here.
type CurrentCmd struct {
	Tools []string `kong:"arg,optional,enum='node,pnpm,yarn,bun',help='Tools to show (default: all that are configured).'"`
}

func (c CurrentCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	tools := c.Tools
	if len(tools) == 0 {
		tools = managedTools
	}
	ctx := context.Background()
	w := tabwriter.NewWriter(g.Stdout, 0, 0, 2, ' ', 0)
	failed := false
	for _, tool := range tools {
		v, ok := s.Cfg.Lookup(versionKey(tool))
		if !ok || v.Value == "" {
			if len(c.Tools) > 0 {
				fmt.Fprintf(w, "%s\t-\tnot configured\n", tool)
			}
			continue
		}
		res, err := s.Res.Quiet(ctx, tool, v.Value)
		if err != nil {
			fmt.Fprintf(w, "%s\t?\t%s %q (could not resolve: %v)\n", tool, v.Source, v.Value, errText(err))
			failed = true
			continue
		}
		why := fmt.Sprintf("%s %q", v.Source, v.Value)
		version := res.Version
		if tool == "node" {
			if raised := minSafe(s, version); raised != version {
				why += fmt.Sprintf(", raised from %s by min-safe-node", version)
				version = raised
			}
		}
		state := ""
		if !installed(s.Root(), tool, version) {
			state = " (not installed)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s%s\n", tool, version, why, state)
	}
	w.Flush()
	if failed {
		return exitCode(1)
	}
	return nil
}

func errText(err error) string {
	if errors.Is(err, resolve.ErrReported) || err.Error() == "" {
		return "see above"
	}
	return err.Error()
}

// WhichCmd prints the binary a tool runs.
type WhichCmd struct {
	Tool string `kong:"arg,enum='node,npm,npx,pnpm,pnpx,yarn,bun,bunx',help='Tool name.'"`
}

func (c WhichCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	s.Out = quietOut(s)
	argv, err := s.Command(context.Background(), c.Tool, nil)
	if err != nil {
		return err
	}
	// node runs the scripts of npm, pnpm and yarn; the script is the interesting part
	fmt.Fprintln(g.Stdout, argv[len(argv)-1])
	return nil
}

// ConfigCmd shows the effective configuration.
type ConfigCmd struct {
	JSON bool `kong:"help='Print JSON.'"`
}

type configEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Source  string `json:"source"`
	Managed string `json:"managed,omitempty"`
}

func (c ConfigCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	var entries []configEntry
	seen := map[string]bool{}
	for _, k := range s.Cfg.Keys() {
		v, _ := s.Cfg.Lookup(k)
		e := configEntry{Key: k, Value: v.Value, Source: v.Source}
		if m, ok := s.Managed.Raw(k); ok {
			e.Managed = m
		}
		entries = append(entries, e)
		seen[k] = true
	}
	for _, k := range s.Managed.Keys() {
		if !seen[k] {
			m, _ := s.Managed.Raw(k)
			entries = append(entries, configEntry{Key: k, Source: "managed profile", Managed: m})
		}
	}
	if c.JSON {
		enc := json.NewEncoder(g.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	w := tabwriter.NewWriter(g.Stdout, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		line := fmt.Sprintf("%s=%s\t%s", e.Key, e.Value, e.Source)
		if e.Managed != "" {
			line += fmt.Sprintf(" (managed profile: %s)", e.Managed)
		}
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

// InstallCmd installs tools ahead of time.
type InstallCmd struct {
	Tools []string `kong:"arg,optional,help='tool or tool@version (node, pnpm, yarn, bun). Default: every tool configured here.'"`
}

func (c InstallCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	ctx := context.Background()
	specs := c.Tools
	if len(specs) == 0 {
		for _, tool := range managedTools {
			if s.Cfg.Get(versionKey(tool)) != "" {
				specs = append(specs, tool)
			}
		}
		if len(specs) == 0 {
			return fmt.Errorf("no tool versions are configured here; pass tool@version")
		}
	}
	for _, spec := range specs {
		tool, version, _ := strings.Cut(spec, "@")
		if !contains(managedTools, tool) {
			return fmt.Errorf("unknown tool %q (expected one of %s)", tool, strings.Join(managedTools, ", "))
		}
		if version != "" {
			os.Setenv(versionKey(tool), version)
			if s, err = session(g); err != nil {
				return err
			}
		}
		var got string
		if tool == "node" {
			got, _, err = s.Node(ctx)
		} else {
			got, err = s.Tool(ctx, tool)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(g.Stdout, "%s %s\n", tool, got)
	}
	return nil
}

// ListCmd lists installed versions.
type ListCmd struct {
	Tools []string `kong:"arg,optional,enum='node,pnpm,yarn,bun',help='Tools to list (default: all).'"`
}

func (c ListCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	tools := c.Tools
	if len(tools) == 0 {
		tools = managedTools
	}
	w := tabwriter.NewWriter(g.Stdout, 0, 0, 2, ' ', 0)
	for _, tool := range tools {
		for _, v := range sortedInstalled(s.Root(), tool) {
			used := "never used"
			if t, ok := lastUsed(s.Root(), tool, v); ok {
				used = "used " + t.Format("2006-01-02")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", tool, v, used)
		}
	}
	return w.Flush()
}

// UninstallCmd removes installed versions.
type UninstallCmd struct {
	Targets []string `kong:"arg,help='tool@version, e.g. node@22.1.0.'"`
}

func (c UninstallCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	for _, t := range c.Targets {
		tool, version, ok := strings.Cut(t, "@")
		if !ok || !contains(managedTools, tool) || !resolve.IsExact(version) {
			return fmt.Errorf("%q is not tool@version with an exact version (e.g. node@22.1.0)", t)
		}
		dir := filepath.Join(s.Root(), tool, version)
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("%s %s is not installed", tool, version)
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Fprintf(g.Stdout, "Removed %s %s\n", tool, version)
	}
	return nil
}

// PruneCmd removes versions that were not used recently.
type PruneCmd struct {
	KeepUsedDays int  `kong:"default='30',help='Keep versions used within this many days.'"`
	DryRun       bool `kong:"short='n',help='Only print what would be removed.'"`
}

func (c PruneCmd) Run(g *Globals) error {
	s, err := session(g)
	if err != nil {
		return err
	}
	// Never prune what this directory is configured to use.
	keep := map[string]bool{}
	for _, tool := range managedTools {
		if spec := s.Cfg.Get(versionKey(tool)); spec != "" {
			if res, err := s.Res.Quiet(context.Background(), tool, spec); err == nil {
				keep[tool+"@"+res.Version] = true
			}
		}
	}
	cutoff := time.Now().AddDate(0, 0, -c.KeepUsedDays)
	removed := 0
	for _, tool := range managedTools {
		for _, v := range sortedInstalled(s.Root(), tool) {
			t, _ := lastUsed(s.Root(), tool, v)
			if keep[tool+"@"+v] || t.After(cutoff) {
				continue
			}
			verb := "Removed"
			if c.DryRun {
				verb = "Would remove"
			} else if err := os.RemoveAll(filepath.Join(s.Root(), tool, v)); err != nil {
				return err
			}
			fmt.Fprintf(g.Stdout, "%s %s %s\n", verb, tool, v)
			removed++
		}
	}
	if removed == 0 {
		fmt.Fprintln(g.Stdout, "Nothing to prune")
	}
	return nil
}

// ExecCmd runs a tool through hnvm.
type ExecCmd struct {
	Tool string   `kong:"arg,enum='node,npm,npx,pnpm,pnpx,yarn,bun,bunx',help='Tool to run.'"`
	Args []string `kong:"arg,optional,passthrough,help='Arguments for the tool.'"`
}

func (c ExecCmd) Run(g *Globals) error {
	return exitCode(shim.Run(c.Tool, c.Args))
}

// ShimsCmd creates the tool symlinks.
type ShimsCmd struct {
	Dir   string `kong:"arg,optional,default='.',type='path',help='Directory to create the symlinks in.'"`
	Force bool   `kong:"short='f',help='Replace existing files.'"`
}

func (c ShimsCmd) Run(g *Globals) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	for _, tool := range shim.Tools {
		link := filepath.Join(c.Dir, tool)
		if target, err := os.Readlink(link); err == nil && target == self {
			continue
		}
		if _, err := os.Lstat(link); err == nil {
			if !c.Force {
				return fmt.Errorf("%s already exists (use --force to replace it)", link)
			}
			if err := os.Remove(link); err != nil {
				return err
			}
		}
		if err := os.Symlink(self, link); err != nil {
			return err
		}
		fmt.Fprintf(g.Stdout, "%s -> %s\n", link, self)
	}
	return nil
}

func minSafe(s *shim.Session, version string) string {
	saved := s.Out
	s.Out = quietOut(s)
	defer func() { s.Out = saved }()
	s.Cfg.Override("HNVM_NODE", version)
	v, _, err := s.NodeVersion(context.Background())
	if err != nil {
		return version
	}
	return v
}

func installed(root, tool, version string) bool {
	switch tool {
	case "node":
		return fileExists(filepath.Join(root, "node", version, "bin", "node"))
	case "pnpm":
		return fileExists(install.PnpmBin(root, version, "pnpm"))
	case "yarn":
		return fileExists(install.YarnBin(root, version))
	case "bun":
		return fileExists(install.BunBin(root, version))
	}
	return false
}

func sortedInstalled(root, tool string) []string {
	vs := resolve.Installed(root, tool)
	sort.Slice(vs, func(i, j int) bool {
		a, aerr := semver.Parse(vs[i])
		b, berr := semver.Parse(vs[j])
		if aerr != nil || berr != nil {
			return vs[i] < vs[j]
		}
		return semver.Compare(a, b) < 0
	})
	return vs
}

// lastUsed approximates when a version last ran by the access time of its main binary, falling back
// to the modification time of the install directory.
func lastUsed(root, tool, version string) (time.Time, bool) {
	dir := filepath.Join(root, tool, version)
	fi, err := os.Stat(dir)
	if err != nil {
		return time.Time{}, false
	}
	t := fi.ModTime()
	if at, ok := accessTime(entryPoint(root, tool, version)); ok && at.After(t) {
		t = at
	}
	return t, true
}

func entryPoint(root, tool, version string) string {
	switch tool {
	case "pnpm":
		return install.PnpmBin(root, version, "pnpm")
	case "yarn":
		return install.YarnBin(root, version)
	case "bun":
		return install.BunBin(root, version)
	}
	return filepath.Join(root, "node", version, "bin", "node")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
