package semver

import (
	"fmt"
	"strings"
)

// Range patterns, transcribed from node-semver's semver.js. Capture group
// numbering in the code below follows the groups in these patterns.
var (
	// Groups: 1 operator, 2 version.
	reComparator = jsRegexp(`^` + patGTLT + `\s*(` + patFull + `)$|^$`)
	// Groups: 1 operator, 2 major, 3 minor, 4 patch, 5 prerelease, 6 build.
	reXRange = jsRegexp(`^` + patGTLT + `\s*` + patXRange + `$`)
	// Groups: 1 major, 2 minor, 3 patch, 4 prerelease, 5 build.
	reTilde = jsRegexp(`^(?:~>?)` + patXRange + `$`)
	reCaret = jsRegexp(`^(?:\^)` + patXRange + `$`)
	// Groups: 1 from, 2-5 from parts, 6 from build, 7 to, 8-11 to parts,
	// 12 to build.
	reHyphen         = jsRegexp(`^\s*(` + patXRange + `)\s+-\s+(` + patXRange + `)\s*$`)
	reStar           = jsRegexp(`(?:<|>)?=?\s*\*`)
	reComparatorTrim = jsRegexp(`(\s*)` + patGTLT + `\s*(` + patLoosePlain + `|` + patXRange + `)`)
	reTildeTrim      = jsRegexp(`(\s*)(?:~>?)\s+`)
	reCaretTrim      = jsRegexp(`(\s*)(?:\^)\s+`)
	reOrSplit        = jsRegexp(`\s*\|\|\s*`)
	reSpaceSplit     = jsRegexp(`\s+`)
)

// Range is a list of alternatives ("||"), each a list of comparators that must
// all hold. The zero Range matches nothing.
type Range struct {
	sets [][]comparator
}

// comparator is one operator and version pair. A nil version is node-semver's
// ANY comparator, which matches every version.
type comparator struct {
	op  string // one of "", "<", "<=", ">", ">="; "=" is stored as ""
	ver *Version
}

// ParseRange parses an npm semver range. It returns an error exactly when
// node-semver's `new Range(s)` would throw. Dist-tags such as "latest" and
// "lts" are invalid ranges.
func ParseRange(s string) (Range, error) {
	var r Range
	for _, part := range reOrSplit.Split(s, -1) {
		set, err := parseRangeSet(part)
		if err != nil {
			return Range{}, fmt.Errorf("semver: invalid range %q: %w", s, err)
		}
		r.sets = append(r.sets, set)
	}
	return r, nil
}

// ValidRange reports whether s is a valid semver range.
func ValidRange(s string) bool {
	_, err := ParseRange(s)
	return err == nil
}

// Satisfies reports whether v is in the range. A prerelease version only
// matches when some comparator in its set names a prerelease on the same
// major.minor.patch tuple.
func (r Range) Satisfies(v Version) bool {
	for _, set := range r.sets {
		if testSet(set, v) {
			return true
		}
	}
	return false
}

// MaxSatisfying returns the highest version in versions that satisfies rng,
// returned as the original string. Entries that do not parse are skipped. ok is
// false when nothing matches or rng is invalid.
func MaxSatisfying(versions []string, rng string) (string, bool) {
	r, err := ParseRange(rng)
	if err != nil {
		return "", false
	}
	var best string
	var bestV Version
	found := false
	for _, s := range versions {
		v, err := parseVersion(s)
		if err != nil || !r.Satisfies(v) {
			continue
		}
		if !found || Compare(bestV, v) < 0 {
			best, bestV, found = s, v, true
		}
	}
	return best, found
}

// FirstSatisfying returns the first entry, in list order, that satisfies rng.
// Entries that do not parse are skipped. ok is false when nothing matches or
// rng is invalid.
func FirstSatisfying(versions []string, rng string) (string, bool) {
	r, err := ParseRange(rng)
	if err != nil {
		return "", false
	}
	for _, s := range versions {
		v, err := parseVersion(s)
		if err == nil && r.Satisfies(v) {
			return s, true
		}
	}
	return "", false
}

func testSet(set []comparator, v Version) bool {
	for _, c := range set {
		if !c.test(v) {
			return false
		}
	}
	if len(v.Prerelease) == 0 {
		return true
	}
	// A prerelease is only allowed when a comparator in this set has a
	// prerelease on the same tuple. For example, ^1.2.3-pr.1 allows 1.2.3-pr.2
	// but not 1.2.4-alpha.
	for _, c := range set {
		if c.ver == nil || len(c.ver.Prerelease) == 0 {
			continue
		}
		if c.ver.Major == v.Major && c.ver.Minor == v.Minor && c.ver.Patch == v.Patch {
			return true
		}
	}
	return false
}

func (c comparator) test(v Version) bool {
	if c.ver == nil {
		return true
	}
	cmp := Compare(v, *c.ver)
	switch c.op {
	case "":
		return cmp == 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	}
	return false
}

// parseRangeSet turns one "||" alternative into comparators, following
// node-semver's Range.prototype.parseRange.
func parseRangeSet(s string) ([]comparator, error) {
	s = jsTrim(s)

	// 1.2.3 - 2.3.4  =>  >=1.2.3 <=2.3.4
	if m := reHyphen.FindStringSubmatch(s); m != nil {
		s = hyphenReplace(m)
	}
	// > 1.2.3  =>  >1.2.3
	s = reComparatorTrim.ReplaceAllString(s, "${1}${2}${3}")
	// ~ 1.2.3  =>  ~1.2.3
	s = reTildeTrim.ReplaceAllString(s, "${1}~")
	// ^ 1.2.3  =>  ^1.2.3
	s = reCaretTrim.ReplaceAllString(s, "${1}^")
	s = strings.Join(reSpaceSplit.Split(s, -1), " ")

	var out []comparator
	for _, tok := range strings.Split(s, " ") {
		expanded := replaceCarets(tok)
		expanded = replaceTildes(expanded)
		expanded = replaceXRanges(expanded)
		expanded = removeFirstStar(jsTrim(expanded))
		for _, part := range reSpaceSplit.Split(expanded, -1) {
			c, err := parseComparator(part)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// parseComparator parses a single comparator such as ">=1.2.3". An empty
// string means "any version".
func parseComparator(s string) (comparator, error) {
	m := reComparator.FindStringSubmatch(s)
	if m == nil {
		return comparator{}, fmt.Errorf("invalid comparator %q", s)
	}
	op := m[1]
	if op == "=" {
		op = ""
	}
	if m[2] == "" {
		return comparator{op: op}, nil
	}
	v, err := parseVersion(m[2])
	if err != nil {
		return comparator{}, err
	}
	return comparator{op: op, ver: &v}, nil
}

// isX reports whether a version component is a wildcard. An empty component
// (a missing part of a partial version) also counts, as in node-semver.
func isX(id string) bool {
	return id == "" || id == "*" || id == "x" || id == "X"
}

// incDecimal returns the decimal string for id plus one. It works on digit
// strings so it cannot overflow. The result is then validated by parseVersion,
// which rejects anything above 2^53-1, matching node-semver's MAX_SAFE_INTEGER
// check.
func incDecimal(id string) string {
	b := []byte(id)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}

// replaceTildes expands ~ ranges in a whitespace-separated list.
func replaceTildes(s string) string {
	return mapTokens(s, replaceTilde)
}

func replaceTilde(tok string) string {
	m := reTilde.FindStringSubmatch(tok)
	if m == nil {
		return tok
	}
	M, mi, p, pr := m[1], m[2], m[3], m[4]
	switch {
	case isX(M):
		return ""
	case isX(mi):
		return ">=" + M + ".0.0 <" + incDecimal(M) + ".0.0"
	case isX(p):
		return ">=" + M + "." + mi + ".0 <" + M + "." + incDecimal(mi) + ".0"
	case pr != "":
		return ">=" + M + "." + mi + "." + p + "-" + pr + " <" + M + "." + incDecimal(mi) + ".0"
	}
	return ">=" + M + "." + mi + "." + p + " <" + M + "." + incDecimal(mi) + ".0"
}

// replaceCarets expands ^ ranges in a whitespace-separated list.
func replaceCarets(s string) string {
	return mapTokens(s, replaceCaret)
}

func replaceCaret(tok string) string {
	m := reCaret.FindStringSubmatch(tok)
	if m == nil {
		return tok
	}
	M, mi, p, pr := m[1], m[2], m[3], m[4]
	switch {
	case isX(M):
		return ""
	case isX(mi):
		return ">=" + M + ".0.0 <" + incDecimal(M) + ".0.0"
	case isX(p):
		if M == "0" {
			return ">=" + M + "." + mi + ".0 <" + M + "." + incDecimal(mi) + ".0"
		}
		return ">=" + M + "." + mi + ".0 <" + incDecimal(M) + ".0.0"
	}
	if pr != "" {
		pr = "-" + pr
	}
	if M == "0" {
		if mi == "0" {
			return ">=" + M + "." + mi + "." + p + pr + " <" + M + "." + mi + "." + incDecimal(p)
		}
		return ">=" + M + "." + mi + "." + p + pr + " <" + M + "." + incDecimal(mi) + ".0"
	}
	return ">=" + M + "." + mi + "." + p + pr + " <" + incDecimal(M) + ".0.0"
}

// replaceXRanges expands x-ranges, "*", and partial versions in a
// whitespace-separated list.
func replaceXRanges(s string) string {
	return mapTokens(s, replaceXRange)
}

func replaceXRange(tok string) string {
	m := reXRange.FindStringSubmatch(tok)
	if m == nil {
		return tok
	}
	gtlt, M, mi, p := m[1], m[2], m[3], m[4]
	xM := isX(M)
	xm := xM || isX(mi)
	xp := xm || isX(p)
	if gtlt == "=" && xp {
		gtlt = ""
	}
	switch {
	case xM:
		if gtlt == ">" || gtlt == "<" {
			// Nothing can satisfy this.
			return "<0.0.0"
		}
		// Everything satisfies this.
		return "*"
	case gtlt != "" && xp:
		if xm {
			mi = "0"
		}
		if xp {
			p = "0"
		}
		switch gtlt {
		case ">":
			// >1 => >=2.0.0, >1.2 => >=1.3.0, >1.2.3 => >=1.2.4
			gtlt = ">="
			if xm {
				M = incDecimal(M)
				mi = "0"
				p = "0"
			} else if xp {
				mi = incDecimal(mi)
				p = "0"
			}
		case "<=":
			// <=0.7.x means <0.8.0, and <=7.x means <8.0.0
			gtlt = "<"
			if xm {
				M = incDecimal(M)
			} else {
				mi = incDecimal(mi)
			}
		}
		return gtlt + M + "." + mi + "." + p
	case xm:
		return ">=" + M + ".0.0 <" + incDecimal(M) + ".0.0"
	case xp:
		return ">=" + M + "." + mi + ".0 <" + M + "." + incDecimal(mi) + ".0"
	}
	return tok
}

// removeFirstStar drops the first "*" and any operator in front of it, as
// node-semver's replaceStars does. The "*" is redundant with the other
// comparators in its set, and "" means "any version".
func removeFirstStar(s string) string {
	loc := reStar.FindStringIndex(s)
	if loc == nil {
		return s
	}
	return s[:loc[0]] + s[loc[1]:]
}

// hyphenReplace rewrites a hyphen range such as "1.2 - 3.4.5" using
// node-semver's rules. Missing parts of the upper bound become exclusive
// bounds, and missing parts of the lower bound become zeros.
func hyphenReplace(m []string) string {
	from, fM, fm, fp := m[1], m[2], m[3], m[4]
	tM, tm, tp, tpr := m[8], m[9], m[10], m[11]
	switch {
	case isX(fM):
		from = ""
	case isX(fm):
		from = ">=" + fM + ".0.0"
	case isX(fp):
		from = ">=" + fM + "." + fm + ".0"
	default:
		from = ">=" + from
	}
	var to string
	switch {
	case isX(tM):
		to = ""
	case isX(tm):
		to = "<" + incDecimal(tM) + ".0.0"
	case isX(tp):
		to = "<" + tM + "." + incDecimal(tm) + ".0"
	case tpr != "":
		to = "<=" + tM + "." + tm + "." + tp + "-" + tpr
	default:
		to = "<=" + m[7]
	}
	return strings.TrimSpace(from + " " + to)
}

// mapTokens applies f to each whitespace-separated token and joins the results
// with single spaces, like node-semver's replaceX helpers.
func mapTokens(s string, f func(string) string) string {
	toks := reSpaceSplit.Split(jsTrim(s), -1)
	for i, t := range toks {
		toks[i] = f(t)
	}
	return strings.Join(toks, " ")
}
