package install

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Installs become visible with one rename, so they are complete once present. Bash extracted pnpm and
// yarn in place and ran them on every call to check, so a pnpm or yarn directory also needs the marker
// the Go installer writes before the rename: a bash install may have stopped part way.
const completeMarker = ".hnvm-complete"

func markComplete(dir string) error {
	return os.WriteFile(filepath.Join(dir, completeMarker), nil, 0o644)
}

// YarnInstalled reports whether yarn version is completely installed.
func YarnInstalled(root, version string) bool {
	return exists(YarnBin(root, version)) && exists(filepath.Join(YarnDir(root, version), completeMarker))
}

// PnpmInstalled reports whether pnpm version is completely installed.
func PnpmInstalled(root, version string) bool {
	return exists(PnpmBin(root, version, "pnpm")) && exists(filepath.Join(PnpmDir(root, version), completeMarker))
}

// YarnDir is $HNVM_PATH/yarn/<version>.
func YarnDir(root, version string) string { return filepath.Join(root, "yarn", version) }

// YarnBin is the script node runs for yarn.
func YarnBin(root, version string) string {
	return filepath.Join(YarnDir(root, version), "bin", "yarn.js")
}

// EnsureYarn installs yarn from {dist}/{version}/yarn-v{version}.tar.gz.
func (e *Env) EnsureYarn(ctx context.Context, version, dist string) error {
	done := func() bool { return YarnInstalled(e.Root, version) }
	if done() {
		return nil
	}
	url := fmt.Sprintf("%s/%s/yarn-v%s.tar.gz", dist, version, version)
	return e.install("yarn", version, done, func(tmp string) error {
		if err := e.validate(ctx, url, "", ""); err != nil {
			return err
		}
		e.Out.Info("Downloading yarn v%s to %s/yarn", version, e.Root)
		body, err := e.get(ctx, url)
		if err != nil {
			return err
		}
		defer body.Close()
		if err := extractTarGz(body, tmp, 1); err != nil {
			return fmt.Errorf("extracting %s: %w", url, err)
		}
		if !exists(filepath.Join(tmp, "bin", "yarn.js")) {
			return fmt.Errorf("%s did not contain bin/yarn.js", url)
		}
		return markComplete(tmp)
	})
}

// PnpmDir is $HNVM_PATH/pnpm/<version>.
func PnpmDir(root, version string) string { return filepath.Join(root, "pnpm", version) }

// PnpmBin returns the script node runs for pnpm (or pnpx). pnpm 6+ uses .cjs, 2.17+ bin/*.js, older lib/bin/*.js.
func PnpmBin(root, version, name string) string {
	dir := PnpmDir(root, version)
	for _, candidate := range []string{"bin/" + name + ".cjs", "bin/" + name + ".js", "lib/bin/" + name + ".js"} {
		if p := filepath.Join(dir, candidate); exists(p) {
			return p
		}
	}
	return filepath.Join(dir, "bin", name+".js")
}

// EnsurePnpm installs pnpm straight from the registry tarball, which is published fully bundled.
// This replaces the vendored pnpm self-installer, and checks the tarball against the packument's
// sha512 integrity.
func (e *Env) EnsurePnpm(ctx context.Context, version, registry string) error {
	done := func() bool { return PnpmInstalled(e.Root, version) }
	if done() {
		return nil
	}
	registry = strings.TrimSuffix(registry, "/")
	return e.install("pnpm", version, done, func(tmp string) error {
		e.Out.Info("Downloading pnpm v%s to %s/pnpm", version, e.Root)

		meta, err := e.packument(ctx, registry+"/pnpm/"+version)
		if err != nil {
			return fmt.Errorf("looking up pnpm %s: %w", version, err)
		}
		tarball := meta.Dist.Tarball
		if tarball == "" {
			tarball = fmt.Sprintf("%s/pnpm/-/pnpm-%s.tgz", registry, version)
		}

		archive, err := os.CreateTemp(filepath.Join(e.Root, ".tmp"), "pnpm-*.tgz")
		if err != nil {
			return err
		}
		defer os.Remove(archive.Name())
		defer archive.Close()

		body, err := e.get(ctx, tarball)
		if err != nil {
			return err
		}
		h := sha512.New()
		_, err = io.Copy(io.MultiWriter(archive, h), body)
		body.Close()
		if err != nil {
			return err
		}
		if want, ok := strings.CutPrefix(meta.Dist.Integrity, "sha512-"); ok {
			if got := base64.StdEncoding.EncodeToString(h.Sum(nil)); got != want {
				return fmt.Errorf("integrity check failed for %s (expected sha512-%s, got sha512-%s)", tarball, want, got)
			}
		}

		if _, err := archive.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if err := extractTarGz(archive, tmp, 1); err != nil {
			return fmt.Errorf("extracting %s: %w", tarball, err)
		}
		return markComplete(tmp)
	})
}

type packument struct {
	Dist struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

func (e *Env) packument(ctx context.Context, url string) (*packument, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	body, err := e.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var p packument
	return &p, json.NewDecoder(body).Decode(&p)
}

// BunDir is $HNVM_PATH/bun/<version>.
func BunDir(root, version string) string { return filepath.Join(root, "bun", version) }

// BunBin is the bun executable.
func BunBin(root, version string) string { return filepath.Join(BunDir(root, version), "bin", "bun") }

// BunTarget is the release asset name, e.g. bun-darwin-aarch64 or bun-linux-x64-musl.
func BunTarget(goos, goarch, variant string) (string, error) {
	if goos != "linux" && goos != "darwin" {
		return "", fmt.Errorf("OS Platform not supported")
	}
	arch := "x64"
	if goarch == "arm64" {
		arch = "aarch64"
	}
	if variant != "" {
		variant = "-" + variant
	}
	return "bun-" + goos + "-" + arch + variant, nil
}

// EnsureBun installs bun from {dist}/bun-v{version}/{target}.zip.
func (e *Env) EnsureBun(ctx context.Context, version, dist, variant string) error {
	done := func() bool { return isExecutable(BunBin(e.Root, version)) }
	if done() {
		return nil
	}
	target, err := BunTarget(runtime.GOOS, runtime.GOARCH, variant)
	if err != nil {
		e.Out.Error("%v", err)
		return ErrReported
	}
	url := fmt.Sprintf("%s/bun-v%s/%s.zip", dist, version, target)
	return e.install("bun", version, done, func(tmp string) error {
		if err := e.validate(ctx, url, "HNVM_BUN_VARIANT", variant); err != nil {
			return err
		}
		e.Out.Info("Downloading bun v%s to %s/bun", version, e.Root)

		// zip needs random access, so download to a file first
		zipPath := filepath.Join(tmp, target+".zip")
		f, err := os.Create(zipPath)
		if err != nil {
			return err
		}
		body, err := e.get(ctx, url)
		if err != nil {
			f.Close()
			return err
		}
		_, err = io.Copy(f, body)
		body.Close()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if err := extractZipFile(zipPath, target+"/bun", filepath.Join(tmp, "bin", "bun")); err != nil {
			return fmt.Errorf("extracting %s: %w", url, err)
		}
		return os.Remove(zipPath)
	})
}
