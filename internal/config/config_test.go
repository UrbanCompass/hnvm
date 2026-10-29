package config

import (
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseRC(t *testing.T) {
	cases := []struct {
		name, in string
		want     map[string]string
	}{
		{"basic", "HNVM_NODE=22.1.0\nHNVM_PNPM=8.0.0", map[string]string{"HNVM_NODE": "22.1.0", "HNVM_PNPM": "8.0.0"}},
		{"comment first line", "# c\nHNVM_NODE=1.0.0", map[string]string{"HNVM_NODE": "1.0.0"}},
		{"tilde", "HNVM_PATH=~/x", map[string]string{"HNVM_PATH": "/h/x"}},
		{"quotes and spaces", "HNVM_NODE_DIST = \"https://a b\" ", map[string]string{"HNVM_NODE_DIST": "https://a b"}},
		{"non-HNVM keys are ignored", "PATH=/evil\nHNVM_NODE=1.0.0\nLD_PRELOAD=x", map[string]string{"HNVM_NODE": "1.0.0"}},
		{"garbage", "nonsense\n=x\nHNVM_=1", map[string]string{}},
		{"crlf", "HNVM_NODE=1.0.0\r\n", map[string]string{"HNVM_NODE": "1.0.0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseRC(c.in, "/h")
			if len(got) != len(c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestPrecedence(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	repo := filepath.Join(root, "repo")
	cwd := filepath.Join(repo, "pkg")
	write(t, filepath.Join(repo, ".git", "HEAD"), "")
	write(t, filepath.Join(home, ".hnvmrc"), "HNVM_NODE=1.0.0\nHNVM_PNPM=1.0.0\nHNVM_YARN=1.0.0\nHNVM_BUN=1.0.0")
	write(t, filepath.Join(repo, ".hnvmrc"), "HNVM_PNPM=2.0.0\nHNVM_YARN=2.0.0\nHNVM_BUN=2.0.0")
	write(t, filepath.Join(cwd, ".hnvmrc"), "HNVM_YARN=3.0.0\nHNVM_BUN=3.0.0")
	write(t, filepath.Join(cwd, "package.json"), `{"engines":{"node":"4.0.0","bun":"4.0.0"},"hnvm":{"bun":"5.0.0"}}`)

	c, err := Load(Inputs{Env: env(map[string]string{"HNVM_BUN": "6.0.0"}), Cwd: cwd, Home: home, DefaultRC: "HNVM_NODE=latest\nHNVM_YARN=latest\nHNVM_PNPM=latest"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Value{
		"HNVM_NODE": {"4.0.0", "package.json engines.node"},
		"HNVM_PNPM": {"2.0.0", root + "/repo/.hnvmrc"},
		"HNVM_YARN": {"3.0.0", "./.hnvmrc"},
		"HNVM_BUN":  {"6.0.0", "environment"},
	}
	for k, w := range want {
		if got, _ := c.Lookup(k); got != w {
			t.Errorf("%s = %+v, want %+v", k, got, w)
		}
	}
}

func TestEnvBeatsRCForNonVersionKeys(t *testing.T) {
	// Documented behavior that the bash implementation got wrong: an rc file overrode the environment.
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".hnvmrc"), "HNVM_QUIET=false\nHNVM_PATH=/from/rc")
	c, _ := Load(Inputs{Env: env(map[string]string{"HNVM_QUIET": "true"}), Cwd: dir, Home: dir + "/h"})
	if c.Get("HNVM_QUIET") != "true" {
		t.Errorf("HNVM_QUIET = %q, want env value", c.Get("HNVM_QUIET"))
	}
	if c.Get("HNVM_PATH") != "/from/rc" {
		t.Errorf("rc should beat the default, got %q", c.Get("HNVM_PATH"))
	}
}

func TestPackageJSONKeyOrder(t *testing.T) {
	cases := []struct{ name, json, want, source string }{
		{"hnvm.node first", `{"hnvm":{"node":"1.0.0"},"engines":{"hnvm":"2.0.0","node":"3.0.0"}}`, "1.0.0", "package.json hnvm.node"},
		{"engines.hnvm second", `{"engines":{"hnvm":"2.0.0","node":"3.0.0"}}`, "2.0.0", "package.json engines.hnvm"},
		{"engines.node last", `{"engines":{"node":">=18"}}`, ">=18", "package.json engines.node"},
		{"numeric", `{"engines":{"node":22}}`, "22", "package.json engines.node"},
		{"null is unset", `{"engines":{"node":null}}`, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "package.json"), c.json)
			cfg, err := Load(Inputs{Env: env(nil), Cwd: dir, Home: dir + "/h"})
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := cfg.Lookup("HNVM_NODE"); got.Value != c.want || got.Source != c.source {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestInvalidPackageJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.json"), "{")
	if _, err := Load(Inputs{Env: env(nil), Cwd: dir, Home: dir}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNoFallback(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cwd := filepath.Join(root, "work")
	write(t, filepath.Join(home, ".hnvmrc"), "HNVM_NODE=1.0.0")
	write(t, filepath.Join(cwd, ".hnvmrc"), "HNVM_NOFALLBACK=true")

	c, _ := Load(Inputs{Env: env(nil), Cwd: cwd, Home: home, DefaultRC: "HNVM_NODE=latest"})
	if got := c.Get("HNVM_NODE"); got != "" {
		t.Errorf("HNVM_NODE = %q, want unset when fallback is disabled", got)
	}
	// Defaults that live outside the rc files still apply.
	if c.Get("HNVM_RANGE_CACHE") != "60" {
		t.Error("built-in defaults must survive HNVM_NOFALLBACK")
	}

	// The environment outranks rc files, so it can turn fallback back on.
	c, _ = Load(Inputs{Env: env(map[string]string{"HNVM_NOFALLBACK": "false"}), Cwd: cwd, Home: home, DefaultRC: "HNVM_NODE=latest"})
	if got := c.Get("HNVM_NODE"); got != "1.0.0" {
		t.Errorf("HNVM_NODE = %q, want 1.0.0 when the environment re-enables fallback", got)
	}
}

func TestGitRootWorktreeFile(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git"), "gitdir: /elsewhere") // a worktree has a .git file
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	if got := GitRoot(sub); got != root {
		t.Errorf("GitRoot = %q, want %q", got, root)
	}
}
