// Package minsafe raises a pinned Node version that has known security problems to the fixed
// release for its line. Rules come from lib/hnvm/min-safe-node.txt, compiled into the binary, so the
// next incident only needs a new line there (and a release).
package minsafe

import (
	"strconv"
	"strings"
)

// Rule maps a pinned major (Below=false: exactly Major; Below=true: every major below Major) to a safe version.
type Rule struct {
	Major int
	Below bool
	Safe  string
}

// ParseRules reads the rules file: "<pinned major> <safe version>" per line, "<N" for every major
// below N, # comments. The first matching rule wins.
func ParseRules(text string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		key, below := fields[0], false
		if strings.HasPrefix(key, "<") {
			key, below = key[1:], true
		}
		major, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		rules = append(rules, Rule{major, below, fields[1]})
	}
	return rules
}

// Settings are HNVM_MIN_SAFE_NODE and HNVM_MIN_SAFE_NODE_MAJOR. Managed values come from the MDM
// profile and win over the environment; ok=false means the profile does not set the key.
type Settings struct {
	SameLine        string // HNVM_MIN_SAFE_NODE (default "true")
	CrossMajor      string // HNVM_MIN_SAFE_NODE_MAJOR (default "false")
	ManagedSameLine func() (bool, bool)
	ManagedMajor    func() (bool, bool)
}

// Result is the version to run, plus the notice to print (empty when there is nothing to say).
type Result struct {
	Version string
	Notice  string
	Raised  bool
}

// Apply returns the version to run for pinned. Anything but an exact version is returned untouched.
// Managed settings are only read once a version needs action, so the common case costs nothing.
func Apply(pinned string, rules []Rule, s Settings) Result {
	res := Result{Version: pinned}
	p, ok := parse(pinned)
	if !ok {
		return res
	}

	safe := ""
	for _, r := range rules {
		if (r.Below && p[0] < r.Major) || (!r.Below && p[0] == r.Major) {
			safe = r.Safe
			break
		}
	}
	sv, ok := parse(safe)
	if !ok {
		return res
	}

	if sv[0] == p[0] {
		if !less(p, sv) {
			return res
		}
		setting := orDefault(s.SameLine, "true") != "false"
		managed, isManaged := call(s.ManagedSameLine)
		if isManaged {
			setting = managed
		}
		if !setting {
			return res
		}
		if isManaged {
			res.Notice = "node " + pinned + " has known security problems; using " + safe + " instead (set by your organization)."
		} else {
			res.Notice = "node " + pinned + " has known security problems; using " + safe + " instead. Set HNVM_MIN_SAFE_NODE=false to opt out."
		}
		res.Version, res.Raised = safe, true
		return res
	}

	setting := orDefault(s.CrossMajor, "false") == "true"
	managed, isManaged := call(s.ManagedMajor)
	if isManaged {
		setting = managed
	}
	switch {
	case setting:
		res.Notice = "node " + pinned + " has known security problems and no fix on its line; using " + safe + " instead."
		res.Version, res.Raised = safe, true
	case isManaged:
		res.Notice = "node " + pinned + " has known security problems and no fix on its line. Move to " + safe + " or newer."
	default:
		res.Notice = "node " + pinned + " has known security problems and no fix on its line. Move to " + safe + " or newer, or set HNVM_MIN_SAFE_NODE_MAJOR=true to have hnvm do it."
	}
	return res
}

func call(f func() (bool, bool)) (bool, bool) {
	if f == nil {
		return false, false
	}
	return f()
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || part == "" || strings.TrimLeft(part, "0123456789") != "" {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
