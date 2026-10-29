package managed

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildBplist assembles a binary plist from raw object bodies, with one-byte offsets and references. It lets the tests
// describe malformed files that a real encoder would never produce.
func buildBplist(objs [][]byte, top byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("bplist00")
	offsets := make([]byte, len(objs))
	for i, o := range objs {
		offsets[i] = byte(buf.Len())
		buf.Write(o)
	}
	tableStart := buf.Len()
	buf.Write(offsets)
	trailer := make([]byte, 32)
	trailer[6] = 1 // offset size
	trailer[7] = 1 // reference size
	binary.BigEndian.PutUint64(trailer[8:], uint64(len(objs)))
	binary.BigEndian.PutUint64(trailer[16:], uint64(top))
	binary.BigEndian.PutUint64(trailer[24:], uint64(tableStart))
	buf.Write(trailer)
	return buf.Bytes()
}

// dictObjects is {"K": "v"}.
func dictObjects() [][]byte {
	return [][]byte{
		{0xD1, 0x01, 0x02}, // dict, one pair: key object 1, value object 2
		{0x51, 'K'},        // ASCII string "K"
		{0x51, 'v'},        // ASCII string "v"
	}
}

func TestHandBuiltBplist(t *testing.T) {
	root, err := parse(buildBplist(dictObjects(), 0))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if root["K"] != "v" {
		t.Errorf("root = %v, want K=v", root)
	}
}

func TestBplistRejects(t *testing.T) {
	valid := buildBplist(dictObjects(), 0)
	tests := []struct {
		name string
		data []byte
	}{
		{"too short", []byte("bplist00")},
		{"wrong magic", append([]byte("bplist01"), valid[8:]...)},
		{"top object out of range", buildBplist(dictObjects(), 9)},
		{"reference cycle", buildBplist([][]byte{{0xA1, 0x00}}, 0)},
		{"reference out of range", buildBplist([][]byte{{0xA1, 0x07}}, 0)},
		{"object runs past the table", buildBplist([][]byte{{0x5F, 0x10, 'x'}}, 0)},
		{"string length past the end", buildBplist([][]byte{{0x5F, 0x11, 0x7F, 'x'}}, 0)},
		{"dictionary key is not a string", buildBplist([][]byte{{0xD1, 0x01, 0x02}, {0x09}, {0x09}}, 0)},
		{"unsupported marker", buildBplist([][]byte{{0x70}}, 0)},
		{"bad integer width", buildBplist([][]byte{{0x14, 0x01, 0x02}}, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parse(tt.data); err == nil {
				t.Error("parse succeeded, want an error")
			}
		})
	}
}

func TestBplistNestingDepthIsLimited(t *testing.T) {
	// Each array holds the next one; the chain is longer than maxNestingDepth.
	const n = 60
	objs := make([][]byte, n)
	for i := 0; i < n-1; i++ {
		objs[i] = []byte{0xA1, byte(i + 1)}
	}
	objs[n-1] = []byte{0xA0}
	if _, err := parseBinary(buildBplist(objs, 0)); err == nil {
		t.Error("a chain deeper than maxNestingDepth parsed")
	}

	// A shorter chain of the same shape is fine.
	const m = 10
	short := make([][]byte, m)
	for i := 0; i < m-1; i++ {
		short[i] = []byte{0xA1, byte(i + 1)}
	}
	short[m-1] = []byte{0xA0}
	if _, err := parseBinary(buildBplist(short, 0)); err != nil {
		t.Errorf("a shallow chain: %v", err)
	}
}

func TestBplistSharedObjectsDecodeOnce(t *testing.T) {
	// Both dictionary values point at the same string object.
	objs := [][]byte{
		{0xD2, 0x01, 0x02, 0x03, 0x03}, // keys: objects 1, 2; values: object 3 twice
		{0x51, 'A'},
		{0x51, 'B'},
		{0x51, 'v'},
	}
	root, err := parse(buildBplist(objs, 0))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if root["A"] != "v" || root["B"] != "v" {
		t.Errorf("root = %v", root)
	}
}

func TestXMLRejects(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"not a plist root", `<?xml version="1.0"?><root/>`},
		{"text between elements", `<plist><dict>junk<key>K</key><string>v</string></dict></plist>`},
		{"key missing", `<plist><dict><string>v</string></dict></plist>`},
		{"unknown element", `<plist><dict><key>K</key><blob/></dict></plist>`},
		{"bad integer", `<plist><dict><key>K</key><integer>x</integer></dict></plist>`},
		{"top level is not a dict", `<plist><string>v</string></plist>`},
		{"unclosed", `<plist><dict><key>K</key><string>v</string>`},
		{"nested too deeply", `<plist>` + strings.Repeat("<array>", 40) + strings.Repeat("</array>", 40) + `</plist>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parse([]byte(tt.doc)); err == nil {
				t.Error("parse succeeded, want an error")
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"profile.bplist", "profile.xml"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add(buildBplist(dictObjects(), 0))
	f.Add([]byte(xmlProfile(`<key>K</key><array><string>a</string><true/></array>`)))
	f.Add([]byte("bplist00"))
	f.Add([]byte("<plist>"))

	f.Fuzz(func(t *testing.T, data []byte) {
		root, err := parse(data)
		if err != nil {
			return
		}
		p := &Prefs{root: root}
		for _, key := range p.Keys() {
			p.Bool(key)
			p.List(key)
			p.Raw(key)
		}
		p.ApplyDistrust("", "", "")
	})
}
