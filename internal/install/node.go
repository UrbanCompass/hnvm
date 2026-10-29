// Package install downloads and unpacks the tools HNVM manages into $HNVM_PATH.
package install

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/UrbanCompass/hnvm/internal/output"
)

// ErrReported means the failure was already printed in full and the caller should only exit.
var ErrReported = errors.New("")

// Node describes one Node install.
type Node struct {
	Version  string // exact, e.g. 22.1.0
	Root     string // $HNVM_PATH
	Dist     string // HNVM_NODE_DIST
	Variant  string // HNVM_NODE_VARIANT
	SkipHead bool   // HNVM_SKIP_URL_VALIDATION
	Out      *output.Output
	Client   *http.Client
}

// Dir is where this version lives: $HNVM_PATH/node/<version>.
func (n *Node) Dir() string { return filepath.Join(n.Root, "node", n.Version) }

// Bin is the node executable.
func (n *Node) Bin() string { return filepath.Join(n.Dir(), "bin", "node") }

// URL is the tarball to download.
func (n *Node) URL() (string, error) {
	platform, arch, err := hostNodeTarget(n.Version)
	if err != nil {
		return "", err
	}
	variant := ""
	if n.Variant != "" {
		variant = "-" + n.Variant
	}
	return fmt.Sprintf("%s/v%s/node-v%s-%s-%s%s.tar.gz", n.Dist, n.Version, n.Version, platform, arch, variant), nil
}

// Ensure installs the version if node is not already executable. Concurrent callers for the same
// version serialize on a file lock, and the install becomes visible with a single rename, so a
// reader never sees a half-extracted tree.
func (n *Node) Ensure(ctx context.Context) error {
	if isExecutable(n.Bin()) {
		return nil
	}

	parent := filepath.Dir(n.Dir())
	// Extraction happens under .tmp, on the same filesystem as the destination so the rename is atomic.
	for _, dir := range []string{parent, filepath.Join(n.Root, ".tmp")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Locks live beside the tool directories, not in them, so `$HNVM_PATH/node` only lists versions.
	lockDir := filepath.Join(n.Root, ".locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(lockDir, "node-"+n.Version+".lock"))
	if err != nil {
		return err
	}
	defer unlock()

	// Whoever held the lock before us may have finished the install.
	if isExecutable(n.Bin()) {
		return nil
	}
	return n.download(ctx, parent)
}

func (n *Node) download(ctx context.Context, parent string) error {
	url, err := n.URL()
	if err != nil {
		return err
	}
	client := n.Client
	if client == nil {
		client = &http.Client{}
	}

	if !n.SkipHead {
		if err := validateURL(ctx, client, url); err != nil {
			n.Out.Error("URL validation failed: %s", url)
			n.Out.Error("The requested package/version may not exist or the URL is incorrect.")
			if n.Variant != "" {
				n.Out.Error("Note: You are using HNVM_NODE_VARIANT='%s'", n.Variant)
				n.Out.Error("This variant may not be available for the requested version/platform.")
			}
			return ErrReported
		}
	}

	n.Out.Info("Downloading node v%s to %s/node", n.Version, n.Root)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}

	tmp, err := os.MkdirTemp(filepath.Join(n.Root, ".tmp"), "node-"+n.Version+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // no-op once renamed

	if err := extractTarGz(resp.Body, tmp, 1); err != nil {
		return fmt.Errorf("extracting %s: %w", url, err)
	}
	if !isExecutable(filepath.Join(tmp, "bin", "node")) {
		return fmt.Errorf("%s did not contain bin/node", url)
	}

	// A previous half-finished install (from the bash implementation, which extracts in place) has to go.
	if err := os.RemoveAll(n.Dir()); err != nil {
		return err
	}
	return os.Rename(tmp, n.Dir())
}

func validateURL(ctx context.Context, client *http.Client, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HEAD %s: %s", url, resp.Status)
	}
	return nil
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}

// lockFile takes an exclusive advisory lock, blocking until it is free. flock is released by the
// kernel if the holder dies, so there are no stale locks.
func lockFile(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// extractTarGz unpacks r into dest, dropping the first `strip` path components. Entries that would
// land outside dest are rejected.
func extractTarGz(r io.Reader, dest string, strip int) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./"), "/")
		if len(parts) <= strip {
			continue
		}
		target := filepath.Join(root, filepath.Join(parts[strip:]...))
		if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			// Node's npm/npx are relative symlinks into lib/node_modules. Reject ones that escape.
			resolved := filepath.Join(filepath.Dir(target), hdr.Linkname)
			if filepath.IsAbs(hdr.Linkname) || !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
				return fmt.Errorf("unsafe symlink in archive: %q -> %q", hdr.Name, hdr.Linkname)
			}
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
}
