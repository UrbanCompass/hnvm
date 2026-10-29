package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/UrbanCompass/hnvm/internal/output"
)

// ErrReported means the failure was already printed in full and the caller should only exit.
var ErrReported = errors.New("")

// Env is what every installer needs.
type Env struct {
	Root     string // $HNVM_PATH
	SkipHead bool   // HNVM_SKIP_URL_VALIDATION
	Out      *output.Output
	Client   *http.Client
}

func (e *Env) client() *http.Client {
	if e.Client != nil {
		return e.Client
	}
	return http.DefaultClient
}

// install runs fill under the lock for tool@version and publishes the directory it filled with a
// single rename, so readers never see a half-extracted tree. done is checked again once the lock is
// held, because whoever held it before may have finished the same install.
func (e *Env) install(tool, version string, done func() bool, fill func(tmp string) error) error {
	dest := filepath.Join(e.Root, tool, version)
	// Extraction happens under .tmp, on the same filesystem as the destination so the rename is atomic.
	// Locks live beside the tool directories, not in them, so `$HNVM_PATH/<tool>` only lists versions.
	for _, dir := range []string{filepath.Dir(dest), filepath.Join(e.Root, ".tmp"), filepath.Join(e.Root, ".locks")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	unlock, err := lockFile(filepath.Join(e.Root, ".locks", tool+"-"+version+".lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if done() {
		return nil
	}

	tmp, err := os.MkdirTemp(filepath.Join(e.Root, ".tmp"), tool+"-"+version+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // no-op once renamed

	if err := fill(tmp); err != nil {
		return err
	}
	// A previous half-finished install (from the bash implementation, which extracts in place) has to go.
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// validate is validate_url from the bash implementation: a HEAD request, with the same messages.
func (e *Env) validate(ctx context.Context, url, variantKey, variant string) error {
	if e.SkipHead {
		return nil
	}
	if err := head(ctx, e.client(), url); err != nil {
		e.Out.Error("URL validation failed: %s", url)
		e.Out.Error("The requested package/version may not exist or the URL is incorrect.")
		if variant != "" {
			e.Out.Error("Note: You are using %s='%s'", variantKey, variant)
			e.Out.Error("This variant may not be available for the requested version/platform.")
		}
		return ErrReported
	}
	return nil
}

func head(ctx context.Context, client *http.Client, url string) error {
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

// get opens url, failing on any non-200 status. The caller closes the body.
func (e *Env) get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
