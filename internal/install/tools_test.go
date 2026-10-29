package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/UrbanCompass/hnvm/internal/output"
)

// tarGz builds a gzipped tar of files (path -> content) under top/.
func tarGz(t *testing.T, top string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: top + name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func newEnv(t *testing.T, srv *httptest.Server) *Env {
	return &Env{Root: t.TempDir(), Out: output.Discard(), Client: srv.Client()}
}

func TestEnsureYarn(t *testing.T) {
	tarball := tarGz(t, "yarn-v1.22.19/", map[string]string{"bin/yarn.js": "// yarn"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1.22.19/yarn-v1.22.19.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Write(tarball)
	}))
	defer srv.Close()
	e := newEnv(t, srv)

	if err := e.EnsureYarn(context.Background(), "1.22.19", srv.URL); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(YarnBin(e.Root, "1.22.19")); string(got) != "// yarn" {
		t.Fatalf("yarn.js = %q", got)
	}
	if err := e.EnsureYarn(context.Background(), "9.9.9", srv.URL); err != ErrReported {
		t.Fatalf("missing version: err = %v, want ErrReported", err)
	}
	if _, err := os.Stat(YarnDir(e.Root, "9.9.9")); !os.IsNotExist(err) {
		t.Fatal("a failed install left a directory behind")
	}
}

func TestEnsurePnpm(t *testing.T) {
	tarball := tarGz(t, "package/", map[string]string{"bin/pnpm.cjs": "// pnpm", "bin/pnpx.cjs": "// pnpx"})
	sum := sha512.Sum512(tarball)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])

	for _, tc := range []struct {
		name      string
		integrity string
		wantErr   bool
	}{
		{"good integrity", integrity, false},
		{"no integrity", "", false},
		{"bad integrity", "sha512-AAAA", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/pnpm/8.15.0":
					fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":%q}}`, srv.URL+"/pnpm/-/pnpm-8.15.0.tgz", tc.integrity)
				case "/pnpm/-/pnpm-8.15.0.tgz":
					w.Write(tarball)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			e := newEnv(t, srv)

			err := e.EnsurePnpm(context.Background(), "8.15.0", srv.URL+"/")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an integrity error")
				}
				if _, err := os.Stat(PnpmDir(e.Root, "8.15.0")); !os.IsNotExist(err) {
					t.Fatal("a failed install left a directory behind")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := PnpmBin(e.Root, "8.15.0", "pnpx"); filepath.Base(got) != "pnpx.cjs" {
				t.Fatalf("PnpmBin = %s", got)
			}
		})
	}
}

func TestPnpmBinLayouts(t *testing.T) {
	for _, rel := range []string{"bin/pnpm.cjs", "bin/pnpm.js", "lib/bin/pnpm.js"} {
		root := t.TempDir()
		p := filepath.Join(PnpmDir(root, "1.0.0"), rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
		if got := PnpmBin(root, "1.0.0", "pnpm"); got != p {
			t.Errorf("%s: PnpmBin = %s", rel, got)
		}
	}
}

func TestBunTarget(t *testing.T) {
	for _, tc := range []struct{ goos, goarch, variant, want string }{
		{"darwin", "arm64", "", "bun-darwin-aarch64"},
		{"linux", "amd64", "", "bun-linux-x64"},
		{"linux", "amd64", "musl", "bun-linux-x64-musl"},
		{"linux", "arm64", "", "bun-linux-aarch64"},
	} {
		if got, err := BunTarget(tc.goos, tc.goarch, tc.variant); err != nil || got != tc.want {
			t.Errorf("BunTarget(%s, %s, %q) = %q, %v; want %q", tc.goos, tc.goarch, tc.variant, got, err, tc.want)
		}
	}
	if _, err := BunTarget("windows", "amd64", ""); err == nil {
		t.Error("windows should not be supported")
	}
}

func TestEnsureBun(t *testing.T) {
	target, err := BunTarget(runtime.GOOS, runtime.GOARCH, "")
	if err != nil {
		t.Skip("unsupported host")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: target + "/bun", Method: zip.Deflate}
	h.SetMode(0o755)
	f, _ := zw.CreateHeader(h)
	f.Write([]byte("#!/bin/sh\necho 1.1.0\n"))
	zw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bun-v1.1.0/"+target+".zip" {
			http.NotFound(w, r)
			return
		}
		w.Write(buf.Bytes())
	}))
	defer srv.Close()
	e := newEnv(t, srv)

	if err := e.EnsureBun(context.Background(), "1.1.0", srv.URL, ""); err != nil {
		t.Fatal(err)
	}
	if !isExecutable(BunBin(e.Root, "1.1.0")) {
		t.Fatal("bun is not executable")
	}
	entries, _ := os.ReadDir(BunDir(e.Root, "1.1.0"))
	if len(entries) != 1 {
		t.Fatalf("the zip was left in the install: %v", entries)
	}
}
