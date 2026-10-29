// Package install downloads and unpacks the tools HNVM manages into $HNVM_PATH.
package install

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/UrbanCompass/hnvm/internal/output"
)

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
// version serialize on a file lock.
func (n *Node) Ensure(ctx context.Context) error {
	done := func() bool { return isExecutable(n.Bin()) }
	if done() {
		return nil
	}
	url, err := n.URL()
	if err != nil {
		n.Out.Error("%v", err)
		return ErrReported
	}
	env := &Env{Root: n.Root, SkipHead: n.SkipHead, Out: n.Out, Client: n.Client}
	return env.install("node", n.Version, done, func(tmp string) error {
		if err := env.validate(ctx, url, "HNVM_NODE_VARIANT", n.Variant); err != nil {
			return err
		}
		n.Out.Info("Downloading node v%s to %s/node", n.Version, n.Root)
		body, err := env.get(ctx, url)
		if err != nil {
			return err
		}
		defer body.Close()
		if err := extractTarGz(body, tmp, 1); err != nil {
			return fmt.Errorf("extracting %s: %w", url, err)
		}
		if !isExecutable(filepath.Join(tmp, "bin", "node")) {
			return fmt.Errorf("%s did not contain bin/node", url)
		}
		return nil
	})
}
