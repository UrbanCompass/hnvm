// Package config resolves HNVM settings from layered sources and remembers which source decided
// each value, so `hnvm config` can explain itself.
//
// Priority, highest first:
//
//  1. environment variables
//  2. package.json in the working directory (versions only: hnvm.<tool> > engines.hnvm > engines.<tool>)
//  3. .hnvmrc in the working directory
//  4. .hnvmrc at the git root
//  5. ~/.hnvmrc
//  6. the .hnvmrc that ships with HNVM
//  7. built-in defaults
//
// Unlike the bash implementation, an .hnvmrc never overrides the environment for non-version keys.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Value is a setting and the source that decided it.
type Value struct {
	Value  string
	Source string
}

// Config is the resolved configuration for one invocation.
type Config struct {
	values map[string]Value
}

// Inputs are everything Load reads from the outside world, so tests can use fixtures.
type Inputs struct {
	Env       func(string) (string, bool)
	Cwd       string
	Home      string
	DefaultRC string
}

// Defaults for keys that must not be blocked by HNVM_NOFALLBACK.
func defaults(home string) map[string]string {
	return map[string]string{
		"HNVM_PATH":                  filepath.Join(home, ".hnvm"),
		"HNVM_RANGE_CACHE":           "60",
		"HNVM_QUIET":                 "false",
		"HNVM_NODE_DIST":             "https://nodejs.org/dist",
		"HNVM_NODE_VARIANT":          "",
		"HNVM_PNPM_REGISTRY":         "https://registry.npmjs.org",
		"HNVM_YARN_DIST":             "https://yarnpkg.com/downloads",
		"HNVM_BUN_DIST":              "https://github.com/oven-sh/bun/releases/download",
		"HNVM_BUN_VARIANT":           "",
		"HNVM_SKIP_URL_VALIDATION":   "false",
		"HNVM_DISTRUST_CERTS":        "true",
		"HNVM_DISTRUST_CERTS_EXTRA":  "",
		"HNVM_DISTRUST_CERTS_STRICT": "false",
		"HNVM_MIN_SAFE_NODE":         "true",
		"HNVM_MIN_SAFE_NODE_MAJOR":   "false",
	}
}

// Load builds the configuration. It fails only when package.json exists but cannot be parsed.
func Load(in Inputs) (*Config, error) {
	c := &Config{values: map[string]Value{}}

	for k, v := range defaults(in.Home) {
		c.values[k] = Value{v, "default"}
	}

	// Lowest priority first, so later layers overwrite earlier ones.
	rcs := c.rcChain(in)
	for i := len(rcs) - 1; i >= 0; i-- {
		for k, v := range rcs[i].values {
			c.values[k] = Value{v, rcs[i].source}
		}
	}

	pkg, err := readPackageJSON(filepath.Join(in.Cwd, "package.json"))
	if err != nil {
		return nil, err
	}
	for k, v := range pkg {
		c.values[k] = Value{v.Value, v.Source}
	}

	// Environment wins over everything.
	for k := range c.values {
		if v, ok := in.Env(k); ok && v != "" {
			c.values[k] = Value{v, "environment"}
		}
	}
	for _, k := range []string{"HNVM_NODE", "HNVM_PNPM", "HNVM_YARN", "HNVM_BUN"} {
		if v, ok := in.Env(k); ok && v != "" {
			c.values[k] = Value{v, "environment"}
		}
	}
	return c, nil
}

// Keys returns every key that has a value, sorted.
func (c *Config) Keys() []string {
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the value of key, or "" when unset.
func (c *Config) Get(key string) string { return c.values[key].Value }

// Lookup returns the value and its source.
func (c *Config) Lookup(key string) (Value, bool) {
	v, ok := c.values[key]
	return v, ok
}

// Override sets key for the rest of this invocation, recorded as an explicit override.
func (c *Config) Override(key, value string) { c.values[key] = Value{value, "override"} }

// Bool reports whether key is exactly "true", which is how every HNVM flag is spelled.
func (c *Config) Bool(key string) bool { return c.Get(key) == "true" }

type rcFile struct {
	source string
	values map[string]string
}

// rcChain returns the rc files that apply, highest priority first. HNVM_NOFALLBACK=true, from the
// environment or an rc file already read, stops the walk.
func (c *Config) rcChain(in Inputs) []rcFile {
	var chain []rcFile
	stop := func() bool {
		if v, ok := in.Env("HNVM_NOFALLBACK"); ok && v == "true" {
			return true
		}
		for _, rc := range chain {
			if rc.values["HNVM_NOFALLBACK"] == "true" {
				return true
			}
		}
		return false
	}

	candidates := []struct{ path, label string }{
		{filepath.Join(in.Cwd, ".hnvmrc"), "./.hnvmrc"},
	}
	if root := GitRoot(in.Cwd); root != "" && root != in.Cwd {
		candidates = append(candidates, struct{ path, label string }{filepath.Join(root, ".hnvmrc"), root + "/.hnvmrc"})
	}
	candidates = append(candidates, struct{ path, label string }{filepath.Join(in.Home, ".hnvmrc"), "~/.hnvmrc"})

	for _, cand := range candidates {
		if stop() {
			return chain
		}
		data, err := os.ReadFile(cand.path)
		if err != nil || strings.TrimSpace(string(data)) == "" {
			continue
		}
		chain = append(chain, rcFile{cand.label, ParseRC(string(data), in.Home)})
	}
	if !stop() {
		chain = append(chain, rcFile{"shipped .hnvmrc", ParseRC(in.DefaultRC, in.Home)})
	}
	return chain
}

// ParseRC reads strict KEY=VALUE lines. Blank lines and # comments are skipped, a leading ~ in a
// value expands to home, and one pair of surrounding quotes is removed. Only HNVM_* keys are
// accepted, so an rc file cannot export arbitrary variables; anything else is ignored.
func ParseRC(text, home string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validKey(key) {
			continue
		}
		val = strings.TrimSpace(val)
		if n := len(val); n >= 2 && (val[0] == '"' || val[0] == '\'') && val[n-1] == val[0] {
			val = val[1 : n-1]
		}
		if val == "~" || strings.HasPrefix(val, "~/") {
			val = home + val[1:]
		}
		out[key] = val
	}
	return out
}

func validKey(k string) bool {
	if !strings.HasPrefix(k, "HNVM_") || len(k) == len("HNVM_") {
		return false
	}
	for _, r := range k {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// GitRoot walks up from dir looking for .git, which is a directory in a clone and a file in a
// worktree. It returns "" outside a repository. No git subprocess is needed.
func GitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

type pkgJSON struct {
	Hnvm    map[string]any `json:"hnvm"`
	Engines map[string]any `json:"engines"`
}

// readPackageJSON returns HNVM_<TOOL> values found in package.json. A missing file is fine; an
// unparseable one is an error, as in bash.
func readPackageJSON(path string) (map[string]Value, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var p pkgJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("An error occurred while parsing package.json")
	}

	out := map[string]Value{}
	for _, tool := range []string{"node", "pnpm", "yarn", "bun"} {
		candidates := []struct {
			v     any
			label string
		}{
			{p.Hnvm[tool], "package.json hnvm." + tool},
		}
		if tool == "node" {
			candidates = append(candidates, struct {
				v     any
				label string
			}{p.Engines["hnvm"], "package.json engines.hnvm"})
		}
		candidates = append(candidates, struct {
			v     any
			label string
		}{p.Engines[tool], "package.json engines." + tool})

		for _, cand := range candidates {
			if s := stringify(cand.v); s != "" {
				out["HNVM_"+strings.ToUpper(tool)] = Value{s, cand.label}
				break
			}
		}
	}
	return out, nil
}

// stringify mirrors `jq -r`: strings as-is, numbers printed, everything else treated as unset.
func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return ""
}
