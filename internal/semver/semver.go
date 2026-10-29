// Package semver implements npm-compatible semantic versions and version
// ranges. Its behavior mirrors node-semver 5.6.0 (vendored as
// lib/hnvm/semver.js) with node-semver's default options: not loose, and
// prereleases only match comparator sets that name a prerelease on the same
// major.minor.patch tuple.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// maxVersionLength matches node-semver's MAX_LENGTH, measured in UTF-16
	// code units like JavaScript's string length.
	maxVersionLength = 256
	// maxSafeInteger is the largest major, minor or patch node-semver accepts.
	maxSafeInteger = 1<<53 - 1
)

// Version is a parsed semantic version. Build metadata is kept but ignored by
// Compare and by range matching.
type Version struct {
	Major, Minor, Patch uint64
	Prerelease          []string
	Build               []string
}

// Grammar transcribed from node-semver's semver.js. Only the strict forms are
// needed here; the loose prerelease form is used by comparator trimming, as in
// node-semver.
const (
	patNumeric    = `0|[1-9]\d*`
	patNonNumeric = `\d*[a-zA-Z-][a-zA-Z0-9-]*`
	patPreIdent   = `(?:` + patNumeric + `|` + patNonNumeric + `)`
	patPreRelease = `(?:-(` + patPreIdent + `(?:\.` + patPreIdent + `)*))`
	patBuildIdent = `[0-9A-Za-z-]+`
	patBuild      = `(?:\+(` + patBuildIdent + `(?:\.` + patBuildIdent + `)*))`
	patMain       = `(` + patNumeric + `)\.(` + patNumeric + `)\.(` + patNumeric + `)`
	patFull       = `v?` + patMain + patPreRelease + `?` + patBuild + `?`

	patLooseNumeric    = `[0-9]+`
	patLooseMain       = `(` + patLooseNumeric + `)\.(` + patLooseNumeric + `)\.(` + patLooseNumeric + `)`
	patLoosePreIdent   = `(?:` + patLooseNumeric + `|` + patNonNumeric + `)`
	patLoosePreRelease = `(?:-?(` + patLoosePreIdent + `(?:\.` + patLoosePreIdent + `)*))`
	patLoosePlain      = `[v=\s]*` + patLooseMain + patLoosePreRelease + `?` + patBuild + `?`

	patGTLT   = `((?:<|>)?=?)`
	patXID    = `0|[1-9]\d*|x|X|\*`
	patXRange = `[v=\s]*(` + patXID + `)(?:\.(` + patXID + `)(?:\.(` + patXID + `)(?:` + patPreRelease + `)?` + patBuild + `?)?)?`
)

// jsSpaceClass lists the characters JavaScript's \s matches. RE2's \s is
// narrower, so patterns are rewritten to use this class.
const jsSpaceClass = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// jsRegexp compiles a pattern written in JavaScript regex syntax, with \s
// widened to JavaScript's whitespace set.
func jsRegexp(pattern string) *regexp.Regexp {
	pattern = strings.ReplaceAll(pattern, `[v=\s]`, `[v=`+jsSpaceClass+`]`)
	pattern = strings.ReplaceAll(pattern, `\s`, `[`+jsSpaceClass+`]`)
	return regexp.MustCompile(pattern)
}

var reFull = jsRegexp(`^` + patFull + `$`)

// isJSSpace reports whether r is trimmed by JavaScript's String.prototype.trim.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func jsTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

// jsLength returns the length of s in UTF-16 code units.
func jsLength(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// Parse parses a strict semantic version such as "1.2.3", "v1.2.3" or
// "1.2.3-beta.1+build.5", following node-semver's valid(). Surrounding
// whitespace, a leading "=" (only accepted as a comparator operator inside
// ranges), leading zeros, numeric components above 2^53-1, and strings longer
// than 256 UTF-16 code units are all rejected.
func Parse(s string) (Version, error) {
	if jsLength(s) > maxVersionLength {
		return Version{}, fmt.Errorf("semver: version longer than %d characters", maxVersionLength)
	}
	m := reFull.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("semver: invalid version %q", s)
	}
	var v Version
	for i, dst := range []*uint64{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.ParseUint(m[i+1], 10, 64)
		if err != nil || n > maxSafeInteger {
			return Version{}, fmt.Errorf("semver: component out of range in %q", s)
		}
		*dst = n
	}
	if m[4] != "" {
		v.Prerelease = strings.Split(m[4], ".")
	}
	if m[5] != "" {
		v.Build = strings.Split(m[5], ".")
	}
	return v, nil
}

// parseForRange parses a version being tested against a range. Surrounding
// whitespace is trimmed first, as node-semver's SemVer constructor does; the
// length limit applies to the untrimmed input.
func parseForRange(s string) (Version, error) {
	if jsLength(s) > maxVersionLength {
		return Version{}, fmt.Errorf("semver: version longer than %d characters", maxVersionLength)
	}
	return Parse(jsTrim(s))
}

// String returns "major.minor.patch", followed by "-prerelease" when the
// version has one. Build metadata is omitted, as in node-semver.
func (v Version) String() string {
	s := strconv.FormatUint(v.Major, 10) + "." + strconv.FormatUint(v.Minor, 10) + "." + strconv.FormatUint(v.Patch, 10)
	if len(v.Prerelease) > 0 {
		s += "-" + strings.Join(v.Prerelease, ".")
	}
	return s
}

// Compare returns -1, 0 or 1 according to semver precedence. Build metadata
// is ignored.
func Compare(a, b Version) int {
	if c := cmpUint(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpUint(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpUint(a.Patch, b.Patch); c != 0 {
		return c
	}
	return comparePrerelease(a.Prerelease, b.Prerelease)
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// comparePrerelease follows node-semver's comparePre: a release sorts after
// any prerelease of the same tuple, and identifiers are compared pairwise.
func comparePrerelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		return compareIdentifiers(a[i], b[i])
	}
	return cmpInt(len(a), len(b))
}

// compareIdentifiers compares prerelease identifiers. Numeric identifiers
// compare numerically and sort below alphanumeric ones.
func compareIdentifiers(a, b string) int {
	an, bn := isNumericIdentifier(a), isNumericIdentifier(b)
	switch {
	case an && bn:
		// Numeric identifiers have no leading zeros, so length and then
		// lexical order is numeric order.
		if len(a) != len(b) {
			return cmpInt(len(a), len(b))
		}
		return strings.Compare(a, b)
	case an:
		return -1
	case bn:
		return 1
	}
	return strings.Compare(a, b)
}

func isNumericIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
