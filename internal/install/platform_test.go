package install

import "testing"

func TestNodeTarget(t *testing.T) {
	cases := []struct {
		goos, goarch, ver, platform, arch string
		wantErr                           bool
	}{
		{"linux", "amd64", "22.1.0", "linux", "x64", false},
		{"linux", "arm64", "22.1.0", "linux", "arm64", false}, // bash gave x64 here
		{"darwin", "arm64", "22.1.0", "darwin", "arm64", false},
		{"darwin", "arm64", "14.18.0", "darwin", "x64", false}, // no arm64 builds before v16
		{"darwin", "arm64", "16.0.0", "darwin", "arm64", false},
		{"darwin", "amd64", "22.1.0", "darwin", "x64", false},
		{"windows", "amd64", "22.1.0", "", "", true},
	}
	for _, c := range cases {
		p, a, err := NodeTarget(c.goos, c.goarch, c.ver)
		if (err != nil) != c.wantErr || p != c.platform || a != c.arch {
			t.Errorf("%v: got %s/%s/%v", c, p, a, err)
		}
	}
}
