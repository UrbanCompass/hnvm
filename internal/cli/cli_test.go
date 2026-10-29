package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// project makes a throwaway project directory and HNVM_PATH and moves into it.
func project(t *testing.T, pkg string) (root string) {
	t.Helper()
	dir := t.TempDir()
	root = filepath.Join(dir, "hnvm")
	proj := filepath.Join(dir, "proj")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "package.json"), []byte(pkg), 0o644)
	t.Chdir(proj)
	t.Setenv("HOME", dir)
	t.Setenv("HNVM_PATH", root)
	for _, k := range []string{"HNVM_NODE", "HNVM_PNPM", "HNVM_YARN", "HNVM_BUN", "HNVM_NOFALLBACK"} {
		t.Setenv(k, "")
	}
	return root
}

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run("1.2.3", args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	if code, out, _ := runCLI(t, "version"); code != 0 || out != "1.2.3\n" {
		t.Fatalf("code %d, out %q", code, out)
	}
}

func TestUsageError(t *testing.T) {
	if code, _, _ := runCLI(t, "no-such-command"); code == 0 {
		t.Fatal("an unknown command succeeded")
	}
}

func TestCurrentShowsSource(t *testing.T) {
	project(t, `{"engines":{"node":"20.1.0"},"hnvm":{"node":"18.20.8"}}`)
	t.Setenv("HNVM_MIN_SAFE_NODE", "false")
	code, out, errOut := runCLI(t, "current", "node")
	if code != 0 || !strings.Contains(out, "18.20.8") || !strings.Contains(out, `package.json hnvm.node "18.20.8"`) {
		t.Fatalf("code %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("expected (not installed): %s", out)
	}
}

func TestConfigEnvironmentWins(t *testing.T) {
	project(t, `{}`)
	t.Setenv("HNVM_QUIET", "true")
	code, out, _ := runCLI(t, "config")
	if code != 0 {
		t.Fatal(code)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "HNVM_QUIET=") {
			if !strings.Contains(line, "true") || !strings.Contains(line, "environment") {
				t.Fatalf("HNVM_QUIET line: %q", line)
			}
			return
		}
	}
	t.Fatalf("no HNVM_QUIET in:\n%s", out)
}

func TestListUninstallPrune(t *testing.T) {
	root := project(t, `{"engines":{"node":"22.1.0"}}`)
	for _, v := range []string{"22.1.0", "18.0.0"} {
		bin := filepath.Join(root, "node", v, "bin", "node")
		os.MkdirAll(filepath.Dir(bin), 0o755)
		os.WriteFile(bin, nil, 0o755)
	}
	os.MkdirAll(filepath.Join(root, "node", "16.0.0"), 0o755)

	_, out, _ := runCLI(t, "list", "node")
	if !strings.Contains(out, "18.0.0") || !strings.Contains(out, "22.1.0") {
		t.Fatalf("list: %s", out)
	}
	if strings.Index(out, "16.0.0") > strings.Index(out, "18.0.0") {
		t.Errorf("list is not sorted by version: %s", out)
	}

	if code, _, _ := runCLI(t, "uninstall", "node@16.0.0"); code != 0 {
		t.Fatal("uninstall failed")
	}
	if code, _, _ := runCLI(t, "uninstall", "node@16.0.0"); code == 0 {
		t.Fatal("uninstalling a missing version succeeded")
	}

	// Everything is "used" just now, so prune with a negative window removes all but the configured version.
	if code, out, errOut := runCLI(t, "prune", "--keep-used-days=-1"); code != 0 || !strings.Contains(out, "Removed node 18.0.0") || strings.Contains(out, "22.1.0") {
		t.Fatalf("prune: code %d\n%s\n%s", code, out, errOut)
	}
}

func TestShims(t *testing.T) {
	dir := t.TempDir()
	if code, _, errOut := runCLI(t, "shims", dir); code != 0 {
		t.Fatal(errOut)
	}
	for _, tool := range []string{"node", "npm", "pnpx", "bunx"} {
		if _, err := os.Readlink(filepath.Join(dir, tool)); err != nil {
			t.Errorf("%s: %v", tool, err)
		}
	}
	// Idempotent
	if code, _, errOut := runCLI(t, "shims", dir); code != 0 {
		t.Fatal(errOut)
	}
}
