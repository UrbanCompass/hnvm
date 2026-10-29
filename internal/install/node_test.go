package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/UrbanCompass/hnvm/internal/output"
)

func fakeNodeTarball(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	top := "node-v" + version + "-linux-x64/"
	add := func(h *tar.Header, body string) {
		h.Name = top + h.Name
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if body != "" {
			tw.Write([]byte(body))
		}
	}
	add(&tar.Header{Name: "bin/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	body := "#!/bin/sh\necho v" + version + "\n"
	add(&tar.Header{Name: "bin/node", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(body))}, body)
	add(&tar.Header{Name: "bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../lib/node_modules/npm/bin/npm-cli.js"}, "")
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func newNode(t *testing.T, srv *httptest.Server, root string) *Node {
	return &Node{Version: "22.1.0", Root: root, Dist: srv.URL, Out: output.New(os.DevNull, false), Client: srv.Client()}
}

func TestEnsureInstallsOnce(t *testing.T) {
	if _, _, err := hostNodeTarget("22.1.0"); err != nil {
		t.Skip("unsupported host")
	}
	platform, arch, _ := hostNodeTarget("22.1.0")
	tarball := fakeNodeTarball(t, "22.1.0")
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/v22.1.0/node-v22.1.0-" + platform + "-" + arch + ".tar.gz"
		if r.URL.Path != want {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			gets.Add(1)
		}
		w.Write(tarball)
	}))
	defer srv.Close()

	root := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- newNode(t, srv, root).Ensure(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if n := gets.Load(); n != 1 {
		t.Errorf("tarball downloaded %d times, want 1", n)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "node"))
	if len(entries) != 1 || entries[0].Name() != "22.1.0" {
		t.Errorf("node dir should list only the version, got %v", entries)
	}
	if tmp, _ := os.ReadDir(filepath.Join(root, ".tmp")); len(tmp) != 0 {
		t.Errorf("leftover temp dirs: %v", tmp)
	}
	if !isExecutable(filepath.Join(root, "node", "22.1.0", "bin", "node")) {
		t.Error("bin/node missing or not executable")
	}
}

func TestEnsureReportsMissingVersion(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	root := t.TempDir()
	err := newNode(t, srv, root).Ensure(context.Background())
	if err != ErrReported {
		t.Fatalf("got %v, want ErrReported", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "node", "22.1.0")); !os.IsNotExist(statErr) {
		t.Error("a failed install must leave no version directory behind")
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "top/../../evil", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1})
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	if err := extractTarGz(&buf, t.TempDir(), 1); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

// Each link passes a lexical check on its own, but b resolves through sub/a to a sibling of dest.
func TestExtractRejectsChainedSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{dest, outside} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range []*tar.Header{
		{Name: "top/other/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "top/sub/a", Typeflag: tar.TypeSymlink, Linkname: "../other"},
		{Name: "top/b", Typeflag: tar.TypeSymlink, Linkname: "sub/a/../../outside"},
	} {
		tw.WriteHeader(h)
	}
	tw.Close()
	gz.Close()

	if err := extractTarGz(&buf, dest, 1); err == nil {
		t.Fatal("expected the escaping symlink to be rejected")
	}
}

// a points at dest's parent, so creating a/victim would replace a file outside dest before the
// final check ever rejects a.
func TestExtractRejectsLinksCreatedThroughLinks(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	victim := filepath.Join(parent, "victim")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range []*tar.Header{
		{Name: "top/a", Typeflag: tar.TypeSymlink, Linkname: ".."},
		{Name: "top/a/victim", Typeflag: tar.TypeSymlink, Linkname: "dest"},
	} {
		tw.WriteHeader(h)
	}
	tw.Close()
	gz.Close()

	if err := extractTarGz(&buf, dest, 1); err == nil {
		t.Fatal("expected the escaping symlink to be rejected")
	}
	if fi, err := os.Lstat(victim); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("file outside dest was touched: %v", err)
	}
}

func TestExtractKeepsInternalSymlinks(t *testing.T) {
	dest := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range []*tar.Header{
		{Name: "top/lib/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "top/sub/a", Typeflag: tar.TypeSymlink, Linkname: "../lib"},
		{Name: "top/bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../sub/a/../lib/npm-cli.js"},
	} {
		tw.WriteHeader(h)
	}
	tw.Close()
	gz.Close()

	if err := extractTarGz(&buf, dest, 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(dest, "bin", "npm")); got != "../sub/a/../lib/npm-cli.js" {
		t.Errorf("bin/npm -> %q", got)
	}
}

// A regular file entry must not be written through a symlink the archive created earlier.
func TestExtractRejectsWritesThroughSymlinks(t *testing.T) {
	dest := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range []*tar.Header{
		{Name: "top/lib/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "top/link", Typeflag: tar.TypeSymlink, Linkname: "lib"},
		{Name: "top/link/file", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1},
	} {
		tw.WriteHeader(h)
	}
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()

	if err := extractTarGz(&buf, dest, 1); err == nil {
		t.Fatal("expected a write through a symlink to be rejected")
	}
}
