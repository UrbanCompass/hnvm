package semver

import (
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type satisfyCase struct {
	rng, version string
	want         bool
}

type boolCase struct {
	in   string
	want bool
}

type compareCase struct {
	a, b string
	want int
}

type listCase struct {
	versions []string
	rng      string
	want     string
}

// Hand-written cases for behavior the node-semver fixtures above do not
// isolate: error messages are not checked, only accept/reject and results.
var handSatisfyCases = []satisfyCase{
	// Prerelease gating: only comparators naming a prerelease on the same tuple open it.
	{"^1.2.3-beta.2", "1.2.3-beta.3", true},
	{"^1.2.3-beta.2", "1.3.0-beta.1", false},
	{"1.2.3-beta", "1.2.3-beta", true},
	{">=1.0.0", "1.2.3-beta", false},
	{">=1.0.0 <2.0.0", "1.0.0-0", false},
	{"1.2.3-0", "1.2.3-0", true},
	{"*", "1.0.0-rc.1", false},
	{"", "0.0.0-0", false},
	{"1.2.3 || 1.2.4-rc.0", "1.2.4-rc.0", true},
	{"1.2.3 || 1.2.4-rc.0", "1.2.4-rc.1", false},
	// Build metadata is ignored.
	{"1.0.0+build.9", "1.0.0+other", true},
	{">=1.0.0+a <1.0.1", "1.0.0+z", true},
	// Upper-bound exclusivity with prereleases.
	{"<2.0.0", "2.0.0-0", false},
	{"<=2.0.0", "2.0.0-0", false},
	{">1.0.0", "1.0.1-alpha", false},
	// Caret and tilde with 0.0.x.
	{"^0.0.0", "0.0.0", true},
	{"^0.0.0", "0.0.1", false},
	{"~0.0", "0.0.9", true},
	{"~0.0", "0.1.0", false},
	// Comparator with x in patch and operator.
	{">=1.2.x", "1.2.0", true},
	{"<1.2.x", "1.1.99", true},
	{"<1.x.5", "0.9.9", true},
	{"<1.x.5", "1.0.0", false},
	// Surrounding whitespace in versions.
	{"1.2.3", "1.2.3", true},
	// Large components near the safe-integer limit.
	{"9007199254740991.0.0", "9007199254740991.0.0", true},
	{"<9007199254740991.0.0", "9007199254740991.0.0", false},
}

var handParseCases = []boolCase{
	{"1.2.3", true},
	{"0.0.0", true},
	{"1.2.3-", false},
	{" 1.2.3", false},
	{"1.2.3\t", false},
	{"=1.2.3", false},
	{"v1.2.3", true},
	{"V1.2.3", false},
	{"1.2.3-rc.1+b", true},
}

var handValidRangeCases = []boolCase{
	{"latest", false},
	{"next", false},
	{"lts", false},
	{"1.2.3 || latest", false},
	{"file:../x", false},
	{"npm:foo@1", false},
	{"git+https://x", false},
}

func TestParseRange_Valid(t *testing.T) {
	for _, c := range validRangeCases {
		if got := ValidRange(c.in); got != c.want {
			t.Errorf("ValidRange(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	for _, c := range handValidRangeCases {
		if got := ValidRange(c.in); got != c.want {
			t.Errorf("ValidRange(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseRange_ErrorIffInvalid(t *testing.T) {
	for _, c := range validRangeCases {
		_, err := ParseRange(c.in)
		if (err == nil) != c.want {
			t.Errorf("ParseRange(%q) err = %v, want valid=%v", c.in, err, c.want)
		}
	}
}

func TestSatisfies(t *testing.T) {
	all := append(append([]satisfyCase{}, satisfyCases...), handSatisfyCases...)
	for _, c := range all {
		r, err := ParseRange(c.rng)
		if err != nil {
			t.Errorf("ParseRange(%q): %v", c.rng, err)
			continue
		}
		v, err := Parse(c.version)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.version, err)
			continue
		}
		if got := r.Satisfies(v); got != c.want {
			t.Errorf("%q satisfies %q = %v, want %v", c.version, c.rng, got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	for _, c := range parseCases {
		_, err := Parse(c.in)
		if (err == nil) != c.want {
			t.Errorf("Parse(%q) err = %v, want valid=%v", c.in, err, c.want)
		}
	}
	for _, c := range handParseCases {
		_, err := Parse(c.in)
		if (err == nil) != c.want {
			t.Errorf("Parse(%q) err = %v, want valid=%v", c.in, err, c.want)
		}
	}
}

func TestParseFields(t *testing.T) {
	v, err := Parse("v1.2.3-beta.1+build.5")
	if err != nil {
		t.Fatal(err)
	}
	if v.Major != 1 || v.Minor != 2 || v.Patch != 3 {
		t.Errorf("components = %d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	if strings.Join(v.Prerelease, ".") != "beta.1" {
		t.Errorf("prerelease = %v", v.Prerelease)
	}
	if strings.Join(v.Build, ".") != "build.5" {
		t.Errorf("build = %v", v.Build)
	}
	if s := v.String(); s != "1.2.3-beta.1" {
		t.Errorf("String() = %q, want build metadata dropped", s)
	}
}

func TestCompare(t *testing.T) {
	for _, c := range compareCases {
		a, err := Parse(c.a)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.a, err)
		}
		b, err := Parse(c.b)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.b, err)
		}
		if got := Compare(a, b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(b, a); got != -c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestMaxSatisfying(t *testing.T) {
	for _, c := range maxSatisfyingCases {
		got, ok := MaxSatisfying(c.versions, c.rng)
		if ok != (c.want != "") || got != c.want {
			t.Errorf("MaxSatisfying(%v, %q) = %q, %v; want %q", c.versions, c.rng, got, ok, c.want)
		}
	}
}

func TestFirstSatisfying(t *testing.T) {
	for _, c := range firstSatisfyingCases {
		got, ok := FirstSatisfying(c.versions, c.rng)
		if ok != (c.want != "") || got != c.want {
			t.Errorf("FirstSatisfying(%v, %q) = %q, %v; want %q", c.versions, c.rng, got, ok, c.want)
		}
	}
}

func TestMaxAndFirstSkipUnparseable(t *testing.T) {
	vs := []string{"not-a-version", "", "1.0.0", "1.0.1"}
	if got, ok := MaxSatisfying(vs, "*"); !ok || got != "1.0.1" {
		t.Errorf("MaxSatisfying = %q, %v", got, ok)
	}
	if got, ok := FirstSatisfying(vs, "*"); !ok || got != "1.0.0" {
		t.Errorf("FirstSatisfying = %q, %v", got, ok)
	}
	if _, ok := MaxSatisfying(vs, "latest"); ok {
		t.Error("MaxSatisfying with dist-tag should report ok=false")
	}
	if _, ok := FirstSatisfying(vs, "lts"); ok {
		t.Error("FirstSatisfying with dist-tag should report ok=false")
	}
}

// TestNodeDifferential compares this package with the vendored node-semver on
// random ranges. It is skipped unless HNVM_SEMVER_DIFF_NODE names a Node
// binary; the committed fixtures above are the offline record of its output.
func TestNodeDifferential(t *testing.T) {
	nodeBin := os.Getenv("HNVM_SEMVER_DIFF_NODE")
	if nodeBin == "" {
		t.Skip("set HNVM_SEMVER_DIFF_NODE to a node binary to run the differential test")
	}
	_, thisFile, _, _ := runtime.Caller(0)
	semverJS, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", "..", "lib", "hnvm", "semver.js"))
	if err != nil {
		t.Fatal(err)
	}

	atoms := []string{"1", "1.2", "1.2.3", "1.x", "1.2.x", "*", "x", "0", "0.0", "0.2.3", "0.0.3",
		"1.2.3-beta", "1.2.3-beta.2", "2.0.0-rc.1", "~1.2", "~1.2.3", "^1.2.3", "^0.2", ">1.2.3",
		">=1.2", "<1.2.3", "<=0.x", "=1.2.3", "v1.2.3", "1.2.3 - 2.0.0", "1.x - 2.x", "~>1.2"}
	pool := []string{"0.0.0", "0.0.3", "0.2.3", "0.2.4", "0.3.0", "1.0.0", "1.2.2", "1.2.3", "1.2.4",
		"1.2.3-beta.1", "1.2.3-beta.2", "1.2.3-beta.3", "1.3.0", "1.9.9", "2.0.0", "2.0.0-rc.1", "2.0.1", "3.0.0-alpha"}

	rnd := rand.New(rand.NewSource(1))
	type query struct {
		Range   string   `json:"range"`
		Version []string `json:"versions"`
	}
	var queries []query
	for i := 0; i < 400; i++ {
		n := 1 + rnd.Intn(3)
		var parts []string
		for j := 0; j < n; j++ {
			parts = append(parts, atoms[rnd.Intn(len(atoms))])
		}
		r := strings.Join(parts, " ")
		if rnd.Intn(4) == 0 {
			r += " || " + atoms[rnd.Intn(len(atoms))]
		}
		queries = append(queries, query{Range: r, Version: pool})
	}
	in, _ := json.Marshal(queries)

	script := `
const semver = require(process.argv[1]);
const qs = JSON.parse(require('fs').readFileSync(0, 'utf8'));
const out = qs.map(q => {
  let valid = true;
  try { new semver.Range(q.range) } catch (_) { valid = false }
  return {valid, sat: valid ? q.versions.map(v => semver.satisfies(v, q.range)) : []};
});
process.stdout.write(JSON.stringify(out));`
	cmd := exec.Command(nodeBin, "-e", script, semverJS)
	cmd.Stdin = strings.NewReader(string(in))
	outBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var results []struct {
		Valid bool   `json:"valid"`
		Sat   []bool `json:"sat"`
	}
	if err := json.Unmarshal(outBytes, &results); err != nil {
		t.Fatalf("decode node output: %v", err)
	}
	if len(results) != len(queries) {
		t.Fatalf("node returned %d results for %d queries", len(results), len(queries))
	}
	for i, q := range queries {
		res := results[i]
		r, err := ParseRange(q.Range)
		if (err == nil) != res.Valid {
			t.Errorf("ValidRange(%q): got valid=%v, node valid=%v", q.Range, err == nil, res.Valid)
			continue
		}
		if !res.Valid {
			continue
		}
		for j, vs := range q.Version {
			v, err := Parse(vs)
			if err != nil {
				t.Fatalf("Parse(%q): %v", vs, err)
			}
			if got := r.Satisfies(v); got != res.Sat[j] {
				t.Errorf("%q satisfies %q = %v, node says %v", vs, q.Range, got, res.Sat[j])
			}
		}
	}
}

// satisfyCases were generated by running node-semver 5.6.0 (lib/hnvm/semver.js)
// under Node 22 with satisfies(version, range), see TestNodeDifferential.
var satisfyCases = []satisfyCase{
	{"1.2.3", "1.2.3", true},
	{"1.2.3", "v1.2.3", true},
	{"1.2.3", "1.2.4", false},
	{"1.2.3", "1.2.2", false},
	{"=1.2.3", "1.2.3", true},
	{"=1.2.3", "1.2.4", false},
	{"=v1.2.3", "1.2.3", true},
	{"=v1.2.3", "1.2.4", false},
	{"v1.2.3", "1.2.3", true},
	{"v1.2.3", "1.2.4", false},
	{"1.2.3 - 2.3.4", "1.2.3", true},
	{"1.2.3 - 2.3.4", "2.3.4", true},
	{"1.2.3 - 2.3.4", "2.3.0", true},
	{"1.2.3 - 2.3.4", "2.3.5", false},
	{"1.2.3 - 2.3.4", "1.2.2", false},
	{"1.2.3 - 2.3.4", "2.4.0", false},
	{"1.2 - 2.3.4", "1.2.0", true},
	{"1.2 - 2.3.4", "1.9.9", true},
	{"1.2 - 2.3.4", "2.3.4", true},
	{"1.2 - 2.3.4", "2.3.5", false},
	{"1.2 - 2.3.4", "1.1.9", false},
	{"1.2.3 - 2.3", "1.2.3", true},
	{"1.2.3 - 2.3", "2.3.0", true},
	{"1.2.3 - 2.3", "2.3.9", true},
	{"1.2.3 - 2.3", "2.4.0", false},
	{"1.2.3 - 2.3", "1.2.2", false},
	{"1.2.3 - 2", "1.2.3", true},
	{"1.2.3 - 2", "2.0.0", true},
	{"1.2.3 - 2", "2.9.9", true},
	{"1.2.3 - 2", "3.0.0", false},
	{"1.2.3 - 2", "1.2.2", false},
	{"1 - 2", "1.0.0", true},
	{"1 - 2", "2.0.0", true},
	{"1 - 2", "2.5.0", true},
	{"1 - 2", "3.0.0", false},
	{"1 - 2", "0.9.9", false},
	{"1.2 - 3.4", "1.2.0", true},
	{"1.2 - 3.4", "3.4.9", true},
	{"1.2 - 3.4", "3.5.0", false},
	{"1.2 - 3.4", "1.1.9", false},
	{"1.x - 2.x", "1.0.0", true},
	{"1.x - 2.x", "2.9.0", true},
	{"1.x - 2.x", "3.0.0", false},
	{"1.x - 2.x", "0.9.0", false},
	{"1.2.3-beta - 2.0.0", "1.2.3-beta", true},
	{"1.2.3-beta - 2.0.0", "1.2.3", true},
	{"1.2.3-beta - 2.0.0", "2.0.0", true},
	{"1.2.3-beta - 2.0.0", "1.2.3-alpha", false},
	{"1.2.3-beta - 2.0.0", "2.0.0-rc.1", false},
	{"1.2.3-beta - 2.0.0", "1.5.0", true},
	{"1.2.3 - 2.3.4-beta.2", "2.3.4-beta.1", true},
	{"1.2.3 - 2.3.4-beta.2", "2.3.4-beta.2", true},
	{"1.2.3 - 2.3.4-beta.2", "2.3.4", false},
	{"1.2.3 - 2.3.4-beta.2", "2.3.4-beta.3", false},
	{"1.2.3 - 2.3.4-beta.2", "1.2.3", true},
	{"v1.2.3 - v2.3.4", "1.2.3", true},
	{"v1.2.3 - v2.3.4", "2.3.4", true},
	{"v1.2.3 - v2.3.4", "2.3.5", false},
	{"x.2.3", "0.0.0", true},
	{"x.2.3", "9.2.3", true},
	{">1.2.3", "1.2.4", true},
	{">1.2.3", "1.2.3", false},
	{">1.2.3", "1.3.0", true},
	{">1.2.3", "2.0.0-beta", false},
	{">=1.2.3", "1.2.3", true},
	{">=1.2.3", "1.2.2", false},
	{">=1.2.3", "2.0.0", true},
	{"<1.2.3", "1.2.2", true},
	{"<1.2.3", "1.2.3", false},
	{"<1.2.3", "1.2.3-beta", false},
	{"<1.2.3", "1.2.2-beta", false},
	{"<=1.2.3", "1.2.3", true},
	{"<=1.2.3", "1.2.4", false},
	{"<=1.2.3", "1.2.2", true},
	{">1.2.3 <1.2.6", "1.2.4", true},
	{">1.2.3 <1.2.6", "1.2.6", false},
	{">1.2.3 <1.2.6", "1.2.5", true},
	{">1.2.3 <1.2.6", "1.2.3", false},
	{">=1.2.3 <2.0.0", "1.2.3", true},
	{">=1.2.3 <2.0.0", "1.9.9", true},
	{">=1.2.3 <2.0.0", "2.0.0", false},
	{">=1.2.3 <2.0.0", "1.2.2", false},
	{"> 1.2.3", "1.2.4", true},
	{"> 1.2.3", "1.2.3", false},
	{">= 1.2.3 < 2.0.0", "1.5.0", true},
	{">= 1.2.3 < 2.0.0", "2.0.0", false},
	{">=1.2.3-beta <1.3.0", "1.2.3-beta.1", true},
	{">=1.2.3-beta <1.3.0", "1.2.3-alpha", false},
	{">=1.2.3-beta <1.3.0", "1.2.4", true},
	{">=1.2.3-beta <1.3.0", "1.2.3-beta", true},
	{">=1.2.3-beta.1 <1.2.3", "1.2.3-beta.2", true},
	{">=1.2.3-beta.1 <1.2.3", "1.2.3-beta.1", true},
	{">=1.2.3-beta.1 <1.2.3", "1.2.3-rc.1", true},
	{">=1.2.3-beta.2", "1.2.3-beta.10", true},
	{">=1.2.3-beta.2", "1.2.3-beta.1", false},
	{">=1.2.3-beta.2", "1.2.3-beta.a", true},
	{">=1.2.3-beta.2", "1.2.3-beta.2", true},
	{"*", "1.2.3", true},
	{"*", "0.0.0", true},
	{"*", "2.0.0-beta", false},
	{"*", "v9.9.9", true},
	{"", "1.0.0", true},
	{"", "0.0.1", true},
	{"", "1.2.3-beta", false},
	{"x", "1.0.0", true},
	{"x", "0.0.1", true},
	{"X.X.X", "3.4.5", true},
	{"1.x", "1.0.0", true},
	{"1.x", "1.9.9", true},
	{"1.x", "2.0.0", false},
	{"1.x", "0.9.9", false},
	{"1.X", "1.5.0", true},
	{"1.X", "2.0.0", false},
	{"1.2.x", "1.2.0", true},
	{"1.2.x", "1.2.9", true},
	{"1.2.x", "1.3.0", false},
	{"1.2.x", "1.1.9", false},
	{"1.2.*", "1.2.0", true},
	{"1.2.*", "1.2.9", true},
	{"1.2.*", "1.3.0", false},
	{"1.*.*", "1.0.0", true},
	{"1.*.*", "1.9.9", true},
	{"1.*.*", "2.0.0", false},
	{"*.*", "0.0.1", true},
	{"*.*", "9.9.9", true},
	{"1", "1.0.0", true},
	{"1", "1.9.9", true},
	{"1", "2.0.0", false},
	{"1", "0.9.9", false},
	{"1.2", "1.2.0", true},
	{"1.2", "1.2.9", true},
	{"1.2", "1.3.0", false},
	{"~1.2.3", "1.2.3", true},
	{"~1.2.3", "1.2.9", true},
	{"~1.2.3", "1.3.0", false},
	{"~1.2.3", "1.2.2", false},
	{"~1.2", "1.2.0", true},
	{"~1.2", "1.2.9", true},
	{"~1.2", "1.3.0", false},
	{"~1.2", "1.1.9", false},
	{"~1", "1.0.0", true},
	{"~1", "1.9.9", true},
	{"~1", "2.0.0", false},
	{"~>1.2.3", "1.2.3", true},
	{"~>1.2.3", "1.3.0", false},
	{"~ 1.2.3", "1.2.4", true},
	{"~ 1.2.3", "1.3.0", false},
	{"~1.2.3-beta.2", "1.2.3-beta.2", true},
	{"~1.2.3-beta.2", "1.2.3-beta.3", true},
	{"~1.2.3-beta.2", "1.2.3", true},
	{"~1.2.3-beta.2", "1.3.0", false},
	{"~1.2.3-beta.2", "1.2.4-beta", false},
	{"~0.0.1", "0.0.1", true},
	{"~0.0.1", "0.0.9", true},
	{"~0.0.1", "0.1.0", false},
	{"^1.2.3", "1.2.3", true},
	{"^1.2.3", "1.9.9", true},
	{"^1.2.3", "2.0.0", false},
	{"^1.2.3", "1.2.2", false},
	{"^0.2.3", "0.2.3", true},
	{"^0.2.3", "0.2.9", true},
	{"^0.2.3", "0.3.0", false},
	{"^0.0.3", "0.0.3", true},
	{"^0.0.3", "0.0.4", false},
	{"^0.0.3", "0.1.0", false},
	{"^0.0", "0.0.0", true},
	{"^0.0", "0.0.9", true},
	{"^0.0", "0.1.0", false},
	{"^0.0.x", "0.0.0", true},
	{"^0.0.x", "0.0.9", true},
	{"^0.0.x", "0.1.0", false},
	{"^0.x", "0.0.1", true},
	{"^0.x", "0.9.0", true},
	{"^0.x", "1.0.0", false},
	{"^1.x", "1.0.0", true},
	{"^1.x", "1.9.9", true},
	{"^1.x", "2.0.0", false},
	{"^1.2", "1.2.0", true},
	{"^1.2", "1.9.0", true},
	{"^1.2", "2.0.0", false},
	{"^1.2.3-beta.2", "1.2.3-beta.2", true},
	{"^1.2.3-beta.2", "1.2.3-beta.3", true},
	{"^1.2.3-beta.2", "1.2.3", true},
	{"^1.2.3-beta.2", "1.9.0", true},
	{"^1.2.3-beta.2", "2.0.0-beta.1", false},
	{"^1.2.3-beta.2", "1.2.4-beta", false},
	{"^0.2.3-beta.2", "0.2.3-beta.2", true},
	{"^0.2.3-beta.2", "0.2.3", true},
	{"^0.2.3-beta.2", "0.2.4-beta", false},
	{"^0.2.3-beta.2", "0.3.0", false},
	{"^0.0.3-beta", "0.0.3-beta", true},
	{"^0.0.3-beta", "0.0.3", true},
	{"^0.0.3-beta", "0.0.4", false},
	{"^ 1.2.3", "1.2.3", true},
	{"^ 1.2.3", "2.0.0", false},
	{"^1.2.3 || ^2.0.0", "1.5.0", true},
	{"^1.2.3 || ^2.0.0", "2.5.0", true},
	{"^1.2.3 || ^2.0.0", "3.0.0", false},
	{"1.2.3 || 2.3.4", "1.2.3", true},
	{"1.2.3 || 2.3.4", "2.3.4", true},
	{"1.2.3 || 2.3.4", "2.3.5", false},
	{"1.2.3 2.3.4", "1.2.3", false},
	{"1.2.3 2.3.4", "2.3.4", false},
	{">=1.0.0 <2.0.0 || >=3.0.0", "1.5.0", true},
	{">=1.0.0 <2.0.0 || >=3.0.0", "2.5.0", false},
	{">=1.0.0 <2.0.0 || >=3.0.0", "3.1.0", true},
	{"|| 1.2.3", "9.9.9", true},
	{"|| 1.2.3", "1.2.3", true},
	{"1.2.3 ||", "0.0.1", true},
	{"||", "0.0.1", true},
	{"||", "9.9.9", true},
	{"<0.0.0", "0.0.0", false},
	{"<0.0.0", "0.0.1", false},
	{"<*", "1.0.0", false},
	{">=*", "1.0.0", true},
	{"=1.x", "1.5.0", true},
	{"=1.x", "2.0.0", false},
	{">=1.x", "1.0.0", true},
	{">=1.x", "0.9.0", false},
	{">=1.x", "2.0.0", true},
	{"<=1.x", "1.9.9", true},
	{"<=1.x", "2.0.0", false},
	{">1.x", "1.9.9", false},
	{">1.x", "2.0.0", true},
	{"<1.x", "0.9.9", true},
	{"<1.x", "1.0.0", false},
	{"<1.2.x", "1.1.9", true},
	{"<1.2.x", "1.2.0", false},
	{">1.2.x", "1.2.9", false},
	{">1.2.x", "1.3.0", true},
	{"<=1.2.x", "1.2.9", true},
	{"<=1.2.x", "1.3.0", false},
	{">=1.2", "1.2.0", true},
	{">=1.2", "1.1.9", false},
	{"<1.2", "1.1.9", true},
	{"<1.2", "1.2.0", false},
	{"<=1.2", "1.2.9", true},
	{"<=1.2", "1.3.0", false},
	{">1.2", "1.2.9", false},
	{">1.2", "1.3.0", true},
	{"1.2.3-beta", "1.2.3-beta", true},
	{"1.2.3-beta", "1.2.3-beta.2", false},
	{"1.2.3-beta", "1.2.3", false},
	{"1.2.3-beta", "1.2.3-alpha", false},
	{"  1.2.3  ", "1.2.3", true},
	{"  1.2.3  ", "1.2.4", false},
	{">= 1.2.3 || <1.0.0", "1.2.3", true},
	{">= 1.2.3 || <1.0.0", "0.5.0", true},
	{">= 1.2.3 || <1.0.0", "1.0.0", false},
	{"1.2.3 - 1.2.3", "1.2.3", true},
	{"1.2.3 - 1.2.3", "1.2.4", false},
	{"1.2.x - 1.2.x", "1.2.0", true},
	{"1.2.x - 1.2.x", "1.2.9", true},
	{"1.2.x - 1.2.x", "1.3.0", false},
	{"1.2.3 - 2.0.0 || 3.x", "1.5.0", true},
	{"1.2.3 - 2.0.0 || 3.x", "3.4.0", true},
	{"1.2.3 - 2.0.0 || 3.x", "4.0.0", false},
	{"~1.2.3 ~1.2.9", "1.2.3", false},
	{"~1.2.3 ~1.2.9", "1.2.9", true},
	{"~1.2.3 ~1.2.9", "1.2.10", true},
	{">=1.2.3 <=1.2.3", "1.2.3", true},
	{">=1.2.3 <=1.2.3", "1.2.4", false},
	{">1.2.3-alpha.3", "1.2.3-alpha.3", false},
	{">1.2.3-alpha.3", "1.2.3-alpha.4", true},
	{">1.2.3-alpha.3", "1.2.3-beta.1", true},
	{">1.2.3-alpha.3", "1.2.4", true},
	{"1.2.3+build.5", "1.2.3", true},
	{"1.2.3+build.5", "1.2.3+other", true},
}

// validRangeCases: ValidRange(s) must equal want (node-semver new Range(s) throws iff !want).
var validRangeCases = []boolCase{
	{"", true},
	{"||", true},
	{"*", true},
	{"x", true},
	{"X", true},
	{"1", true},
	{"1.2", true},
	{"1.x", true},
	{"1.2.x", true},
	{"1.2.3", true},
	{"=1.2.3", true},
	{"v1.2.3", true},
	{"=v1.2.3", true},
	{"~1.2.3", true},
	{"~>1.2.3", true},
	{"^1.2.3", true},
	{"~1", true},
	{"^0.x", true},
	{">=1.2.3 <2.0.0", true},
	{"1.2.3 - 2.3.4", true},
	{"1.x - 2.x", true},
	{"1.2.3 || 2.3.4", true},
	{">1.2.x", true},
	{"<=1.2", true},
	{" 1.2.3 ", true},
	{"1.2.3-beta.1", true},
	{"1.2.3+b.5", true},
	{">=1.2.3-beta", true},
	{"1.2.3 ||", true},
	{"|| 1.2.3", true},
	{"^1.2.3 - 2.0.0", false},
	{"x.2.3", true},
	{"<*", true},
	{">=*", true},
	{"01.x", false},
	{"latest", false},
	{"lts", false},
	{"next", false},
	{"beta", false},
	{"1.2.3.4", false},
	{"01.2.3", false},
	{"1.02.3", false},
	{"1.2.3-", false},
	{"1..2", false},
	{">>1.2.3", false},
	{"~~1.2.3", false},
	{"^^1.2.3", false},
	{"1.2.3 - ", false},
	{"- 1.2.3", false},
	{"a.b.c", false},
	{"v", false},
	{"vv1.2.3", false},
	{">=", false},
	{"<", false},
	{"<>1.2.3", false},
	{"=>1.2.3", false},
	{"!1.2.3", false},
	{"1.2.3-beta..1", false},
	{"1.2.3+", false},
	{"1.2.3 - 2.3.4 - 3.0.0", false},
	{"1.2.3 || latest", false},
	{"1.2.3 ||| 2.0.0", false},
	{"1.2.3 & 2", false},
	{"npm:foo@1", false},
	{"file:../x", false},
	{"git+https://x", false},
	{"9007199254740992.0.0", false},
	{">=9007199254740992.0.0", false},
	{"1.2.3-01", false},
	{"=1.2.3-01", false},
	{"~>~1.2.3", false},
	{"1.2.3e", false},
	{"1.2.3.x", false},
	{"1.2.3 1.2.4 -", false},
	{">=1.2.3 <", false},
	{"1.2.3-alpha.01", false},
	{"~1.2.3-01", false},
	{"^1.2.3-", false},
	{"x.y.z", false},
	{"1.2.3 - 2.3.4 - ", false},
	{"latest || 1.2.3", false},
	{"1.2.3 foo", false},
	{"^1.2.x-beta.", false},
	{"<=v", false},
	{"1.2.3 ~", false},
	{"~ ", false},
	{"^ ", false},
	{"- ", false},
}

var compareCases = []compareCase{
	{"1.2.3", "1.2.4", -1},
	{"1.2.3-alpha", "1.2.3", -1},
	{"1.2.3-alpha", "1.2.3-alpha.1", -1},
	{"1.2.3-alpha.1", "1.2.3-alpha.beta", -1},
	{"1.2.3-alpha.beta", "1.2.3-beta", -1},
	{"1.2.3-beta.2", "1.2.3-beta.11", -1},
	{"1.2.3-rc.1", "1.2.3-rc.1+build", 0},
	{"1.0.0+a", "1.0.0+b", 0},
	{"2.0.0", "10.0.0", -1},
	{"9007199254740991.0.0", "9007199254740990.0.0", 1},
	{"1.2.3", "v1.2.3", 0},
	{"1.2.3-x.7.z.92", "1.2.3-x.7.z.92", 0},
	{"1.2.3-x.7.z", "1.2.3-x.7.z.92", -1},
	{"1.0.0-alpha-a.b-c-somethinglong+build.1-aef.1-its-okay", "1.0.0-rc.1+build.1", -1},
	{"1.2.3-1", "1.2.3-a", -1},
	{"1.2.3-1", "1.2.3-2", -1},
	{"1.2.3-10", "1.2.3-9", 1},
	{"1.0.0", "1.0.0-0", 1},
	{"0.9.9", "1.0.0", -1},
	{"1.10.0", "1.9.0", 1},
	{"1.0.10", "1.0.9", 1},
	{"1.0.0-rc.1", "1.0.0-rc.1", 0},
	{"2.0.0-beta.1+x", "2.0.0-beta.1+y", 0},
}

var parseCases = []boolCase{
	{"1.2.3", true},
	{"v1.2.3", true},
	{"=1.2.3", false},
	{" 1.2.3 ", false},
	{"1.2.3\n", false},
	{"V1.2.3", false},
	{"1.2", false},
	{"1.2.3.4", false},
	{"01.2.3", false},
	{"1.02.3", false},
	{"1.2.03", false},
	{"1.2.3-", false},
	{"1.2.3-01", false},
	{"1.2.3-alpha.01", false},
	{"1.2.3-alpha.1", true},
	{"1.2.3+build.01", true},
	{"1.2.3+", false},
	{"1.2.3-beta+exp.sha.5114f85", true},
	{"1.2.3-a..b", false},
	{"9007199254740991.0.0", true},
	{"9007199254740992.0.0", false},
	{"1.9007199254740992.0", false},
	{"1.2.3 beta", false},
	{"a.b.c", false},
	{"", false},
	{"v", false},
	{"1.2.3-x-y-z.--", true},
	{"1.2.3----RC-SNAPSHOT.12.9.1--.12+788", true},
	{"1.2.3-0", true},
	{"1.2.3-0.3.7", true},
	{"1.2.3-x.7.z.92", true},
	{"1.2.3-r2", true},
	{"1.2.3-x.Y.z", true},
	{"1.2.3e1", false},
	{"1.2.3+build", true},
	{"0.0.0", true},
	{"vv1.2.3", false},
	{"1.2.3-beta.10", true},
}

var maxSatisfyingCases = []listCase{
	{[]string{"1.2.3", "1.2.4", "1.3.0-beta.1", "2.0.0", "1.2.3-beta.1", "v1.9.0"}, "^1.2.3", "v1.9.0"},
	{[]string{"1.2.3", "1.2.4", "1.3.0-beta.1", "2.0.0", "1.2.3-beta.1", "v1.9.0"}, "~1.2", "1.2.4"},
	{[]string{"1.2.3", "1.2.4", "1.3.0-beta.1", "2.0.0", "1.2.3-beta.1", "v1.9.0"}, "^1.3.0-beta.0", "v1.9.0"},
	{[]string{"1.2.3", "1.2.4", "1.3.0-beta.1", "2.0.0"}, "latest", ""},
	{[]string{"1.2.3", "1.2.4"}, "9.x", ""},
	{[]string{"1.2.3", "1.2.4", "2.0.0"}, "", "2.0.0"},
	{[]string{"1.0.0+a", "1.0.0+b"}, "1.0.0", "1.0.0+a"},
	{[]string{"1.2.3", "1.2.4"}, ">=1.2.3 <1.2.4", "1.2.3"},
}

var firstSatisfyingCases = []listCase{
	{[]string{"2.0.0", "1.9.0", "1.2.4", "1.2.3"}, "^1.2.3", "1.9.0"},
	{[]string{"1.2.3", "1.2.4", "v1.9.0"}, "^1.2.3", "1.2.3"},
	{[]string{"1.0.0+a", "1.0.0+b"}, "1.0.0", "1.0.0+a"},
	{[]string{"2.0.0", "1.2.3"}, "lts", ""},
	{[]string{"3.0.0", "2.0.0-beta.1", "1.5.0"}, "^2.0.0", ""},
	{[]string{"1.2.3", "1.2.4"}, "9.x", ""},
}
