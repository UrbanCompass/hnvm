package managed

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	fpA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fpB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// xmlProfile wraps dictionary body (the <key>/<value> pairs) in a plist document.
func xmlProfile(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>` + body + `</dict></plist>`
}

func xmlPrefs(t *testing.T, body string) *Prefs {
	t.Helper()
	root, err := parse([]byte(xmlProfile(body)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return &Prefs{root: root}
}

func str(key, value string) string { return "<key>" + key + "</key><string>" + value + "</string>" }
func boolean(key string, v bool) string {
	if v {
		return "<key>" + key + "</key><true/>"
	}
	return "<key>" + key + "</key><false/>"
}
func array(key string, elems ...string) string {
	var b strings.Builder
	b.WriteString("<key>" + key + "</key><array>")
	for _, e := range elems {
		b.WriteString("<string>" + e + "</string>")
	}
	b.WriteString("</array>")
	return b.String()
}

func TestBool(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		key     string
		wantVal bool
		wantOK  bool
	}{
		{"boolean true", boolean("K", true), "K", true, true},
		{"boolean false", boolean("K", false), "K", false, true},
		{"string 1", str("K", "1"), "K", true, true},
		{"string true, any case", str("K", "TRUE"), "K", true, true},
		{"string yes", str("K", "Yes"), "K", true, true},
		{"string 0", str("K", "0"), "K", false, true},
		{"string false", str("K", "false"), "K", false, true},
		{"string no", str("K", "NO"), "K", false, true},
		{"integer 1", "<key>K</key><integer>1</integer>", "K", true, true},
		{"integer 0", "<key>K</key><integer>0</integer>", "K", false, true},
		{"real 1.0 prints as 1", "<key>K</key><real>1.0</real>", "K", true, true},
		{"string maybe is not a boolean", str("K", "maybe"), "K", false, false},
		{"integer 2 is not a boolean", "<key>K</key><integer>2</integer>", "K", false, false},
		{"array is not a boolean", array("K", "true"), "K", false, false},
		{"empty string is not a boolean", str("K", ""), "K", false, false},
		{"unset key", str("OTHER", "true"), "K", false, false},
		{"empty profile", "", "K", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := xmlPrefs(t, tt.body).Bool(tt.key)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Bool(%q) = (%v, %v), want (%v, %v)", tt.key, v, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		want   string
		wantOK bool
	}{
		{"string passes through", str("K", fpA+","+fpB), fpA + "," + fpB, true},
		{"array is joined with commas", array("K", fpA, "sha256 Fingerprint="+fpB), fpA + ",sha256 Fingerprint=" + fpB, true},
		{"empty array is set and adds nothing", "<key>K</key><array/>", "", true},
		{"empty string is set and adds nothing", str("K", ""), "", true},
		{"unset key", str("OTHER", fpA), "", false},
		{"dictionary is not a list", "<key>K</key><dict><key>a</key><string>b</string></dict>", "", false},
		{"boolean as a list", boolean("K", true), "true", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := xmlPrefs(t, tt.body).List("K")
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("List(K) = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRaw(t *testing.T) {
	p := xmlPrefs(t, boolean("B", true)+
		"<key>I</key><integer>3</integer>"+
		array("A", fpA, "sha256 Fingerprint="+fpB)+
		"<key>E</key><array/>"+
		"<key>D</key><dict><key>x</key><string>y</string></dict>")
	tests := []struct {
		key    string
		want   string
		wantOK bool
	}{
		{"B", "true", true},
		{"I", "3", true},
		{"A", "(" + fpA + ", sha256 Fingerprint=" + fpB + ")", true},
		{"E", "()", true},
		{"D", "", false},
		{"NOPE", "", false},
	}
	for _, tt := range tests {
		got, ok := p.Raw(tt.key)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("Raw(%q) = (%q, %v), want (%q, %v)", tt.key, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestKeysAreSorted(t *testing.T) {
	p := xmlPrefs(t, str("Z", "1")+str("A", "2")+str("M", "3"))
	if got, want := p.Keys(), []string{"A", "M", "Z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

// The min-safe-node settings are read the same way: a boolean, a string in either case, a non-boolean, and absent keys.
func TestMinSafeNodeBooleans(t *testing.T) {
	p := xmlPrefs(t, boolean("HNVM_MIN_SAFE_NODE", false)+
		str("HNVM_MIN_SAFE_NODE_MAJOR", "TRUE")+
		str("SOMETHING_ELSE", "maybe"))
	tests := []struct {
		key     string
		wantVal bool
		wantOK  bool
	}{
		{"HNVM_MIN_SAFE_NODE", false, true},
		{"HNVM_MIN_SAFE_NODE_MAJOR", true, true},
		{"SOMETHING_ELSE", false, false},
		{"NOT_THERE", false, false},
	}
	for _, tt := range tests {
		if v, ok := p.Bool(tt.key); v != tt.wantVal || ok != tt.wantOK {
			t.Errorf("Bool(%q) = (%v, %v), want (%v, %v)", tt.key, v, ok, tt.wantVal, tt.wantOK)
		}
	}
}

func TestMissingFileReadsAsNoProfile(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "missing.plist"))
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	if v, ok := p.Bool("HNVM_MIN_SAFE_NODE"); ok {
		t.Errorf("Bool on missing file = (%v, true), want unset", v)
	}
	if len(p.Keys()) != 0 {
		t.Errorf("Keys on missing file = %v, want none", p.Keys())
	}
}

func TestNilPrefsIsNoProfile(t *testing.T) {
	var p *Prefs
	if _, ok := p.Bool("K"); ok {
		t.Error("Bool on nil Prefs reported a value")
	}
	if _, ok := p.List("K"); ok {
		t.Error("List on nil Prefs reported a value")
	}
	if _, ok := p.Raw("K"); ok {
		t.Error("Raw on nil Prefs reported a value")
	}
	if len(p.Keys()) != 0 {
		t.Error("Keys on nil Prefs reported keys")
	}
	if got := p.ApplyDistrust("false", "", "true"); got.Enabled != "false" || got.Strict != "true" || got.EnabledManaged {
		t.Errorf("ApplyDistrust on nil Prefs = %+v, want the inputs unchanged", got)
	}
}

func TestLoadCorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.plist")
	if err := os.WriteFile(path, []byte("not a plist at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load of a corrupt file returned no error")
	}
	if p := loadOrEmpty(path); len(p.Keys()) != 0 {
		t.Errorf("loadOrEmpty of a corrupt file = %v, want no profile", p.Keys())
	}
}

func TestLoadDirectoryIsAnError(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("Load of a directory returned no error")
	}
}

func TestLoadDefaultOffMacIsEmpty(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("LoadDefault reads the real profile on macOS")
	}
	if p := LoadDefault(); len(p.Keys()) != 0 {
		t.Errorf("LoadDefault on %s = %v, want no profile", runtime.GOOS, p.Keys())
	}
}

func TestDefaultPath(t *testing.T) {
	if DefaultPath != "/Library/Managed Preferences/com.compass.hnvm.plist" {
		t.Errorf("DefaultPath = %q", DefaultPath)
	}
}

// The distrust cases port managed (MDM) distrust settings from test/managed-prefs.test.js.
func TestApplyDistrust(t *testing.T) {
	tests := []struct {
		name                   string
		body                   string
		enabled, extra, strict string
		want                   DistrustSettings
	}{
		{
			name:    "leave the environment in charge when the profile sets none of them",
			body:    "",
			enabled: "false", extra: fpA, strict: "true",
			want: DistrustSettings{Enabled: "false", Extra: fpA, Strict: "true"},
		},
		{
			name:    "managed booleans win over the environment, and the environment's fingerprints follow the managed ones",
			body:    boolean("HNVM_DISTRUST_CERTS", true) + str("HNVM_DISTRUST_CERTS_EXTRA", fpB) + str("HNVM_DISTRUST_CERTS_STRICT", "YES"),
			enabled: "false", extra: fpA, strict: "false",
			want: DistrustSettings{
				Enabled: "true", EnabledManaged: true,
				Extra: fpB + "," + fpA, ExtraManaged: true,
				Strict: "true", StrictManaged: true,
			},
		},
		{
			name: "managed fingerprints are used alone when the environment has none",
			body: str("HNVM_DISTRUST_CERTS_EXTRA", fpB),
			want: DistrustSettings{Extra: fpB, ExtraManaged: true},
		},
		{
			name:    "a managed false turns distrusting off, whatever the environment says",
			body:    str("HNVM_DISTRUST_CERTS", "0"),
			enabled: "true",
			want:    DistrustSettings{Enabled: "false", EnabledManaged: true},
		},
		{
			name:    "a value that is not a boolean is ignored",
			body:    str("HNVM_DISTRUST_CERTS", "maybe") + str("HNVM_DISTRUST_CERTS_STRICT", "maybe"),
			enabled: "false",
			want:    DistrustSettings{Enabled: "false"},
		},
		{
			name:  "an empty array adds nothing and leaves the environment's fingerprints alone",
			body:  "<key>HNVM_DISTRUST_CERTS_EXTRA</key><array/>",
			extra: fpA,
			want:  DistrustSettings{Extra: fpA},
		},
		{
			name:  "an empty string adds nothing and leaves the environment's fingerprints alone",
			body:  str("HNVM_DISTRUST_CERTS_EXTRA", ""),
			extra: fpA,
			want:  DistrustSettings{Extra: fpA},
		},
		{
			name:  "leave the environment in charge for a key the profile does not set",
			body:  str("HNVM_DISTRUST_CERTS_STRICT", "true"),
			extra: fpA,
			want:  DistrustSettings{Extra: fpA, Strict: "true", StrictManaged: true},
		},
		{
			name: "an array of fingerprints is joined as printed by defaults",
			body: array("HNVM_DISTRUST_CERTS_EXTRA", fpA, "sha256 Fingerprint="+fpB),
			want: DistrustSettings{Extra: fpA + ",sha256 Fingerprint=" + fpB, ExtraManaged: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xmlPrefs(t, tt.body).ApplyDistrust(tt.enabled, tt.extra, tt.strict)
			if got != tt.want {
				t.Errorf("ApplyDistrust = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// profile.xml and profile.bplist hold the same dictionary, written by python3's plistlib. Both must decode to it.
func TestFixturesDecodeAlike(t *testing.T) {
	xmlData, err := os.ReadFile(filepath.Join("testdata", "profile.xml"))
	if err != nil {
		t.Fatal(err)
	}
	binData, err := os.ReadFile(filepath.Join("testdata", "profile.bplist"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(binData, []byte("bplist00")) {
		t.Fatal("profile.bplist is not a binary plist")
	}
	fromXML, err := parse(xmlData)
	if err != nil {
		t.Fatalf("parse profile.xml: %v", err)
	}
	fromBin, err := parse(binData)
	if err != nil {
		t.Fatalf("parse profile.bplist: %v", err)
	}
	if !reflect.DeepEqual(fromXML, fromBin) {
		t.Errorf("binary and XML fixtures decode differently:\nxml: %v\nbin: %v", fromXML, fromBin)
	}
}

func TestBinaryFixtureValues(t *testing.T) {
	p, err := Load(filepath.Join("testdata", "profile.bplist"))
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := p.Bool("HNVM_DISTRUST_CERTS"); !v || !ok {
		t.Errorf("HNVM_DISTRUST_CERTS = (%v, %v), want (true, true)", v, ok)
	}
	if v, ok := p.Bool("HNVM_DISTRUST_CERTS_STRICT"); !v || !ok {
		t.Errorf("HNVM_DISTRUST_CERTS_STRICT = (%v, %v), want (true, true)", v, ok)
	}
	if v, ok := p.Bool("HNVM_MIN_SAFE_NODE"); v || !ok {
		t.Errorf("HNVM_MIN_SAFE_NODE = (%v, %v), want (false, true)", v, ok)
	}
	if v, ok := p.Bool("HNVM_MIN_SAFE_NODE_MAJOR"); !v || !ok {
		t.Errorf("HNVM_MIN_SAFE_NODE_MAJOR = (%v, %v), want (true, true)", v, ok)
	}
	if _, ok := p.Bool("SOMETHING_ELSE"); ok {
		t.Error("SOMETHING_ELSE (maybe) read as a boolean")
	}
	if _, ok := p.Bool("NON_EXISTENT"); ok {
		t.Error("unset key read as a boolean")
	}
	if got, ok := p.List("HNVM_DISTRUST_CERTS_EXTRA"); !ok || got != strings.Repeat("a", 64)+",sha256 Fingerprint="+strings.Repeat("b", 64) {
		t.Errorf("HNVM_DISTRUST_CERTS_EXTRA = (%q, %v)", got, ok)
	}
	if got, ok := p.List("EMPTY_LIST"); got != "" || !ok {
		t.Errorf("EMPTY_LIST = (%q, %v), want (\"\", true)", got, ok)
	}
	if got, ok := p.List("EMPTY_STRING"); got != "" || !ok {
		t.Errorf("EMPTY_STRING = (%q, %v), want (\"\", true)", got, ok)
	}
	for key, want := range map[string]string{
		"COUNT":     "3",
		"BIG":       "1099511627776",
		"NEGATIVE":  "-7",
		"RATIO":     "1.5",
		"ONE_REAL":  "1",
		"ONE_INT":   "1",
		"NON_ASCII": "café",
	} {
		if got, ok := p.Raw(key); !ok || got != want {
			t.Errorf("Raw(%s) = (%q, %v), want (%q, true)", key, got, ok, want)
		}
	}
	if v, ok := p.Bool("ONE_INT"); !v || !ok {
		t.Errorf("integer 1 as Bool = (%v, %v)", v, ok)
	}
	for _, key := range []string{"WHEN", "BLOB"} {
		if _, ok := p.Raw(key); ok {
			t.Errorf("Raw(%s) reported a value for a date or data", key)
		}
	}
	if _, ok := p.Raw("NESTED"); ok {
		t.Error("Raw(NESTED) reported a value for a dictionary")
	}
	if _, ok := p.Bool("NESTED"); ok {
		t.Error("Bool(NESTED) reported a value for a dictionary")
	}
	if got := p.ApplyDistrust("false", "", "false"); got.Enabled != "true" || got.Strict != "true" || !got.EnabledManaged || !got.StrictManaged {
		t.Errorf("ApplyDistrust on the binary fixture = %+v", got)
	}
}
