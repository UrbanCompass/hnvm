// Package shim builds the exec plan for each managed tool.
package shim

import (
	"context"
	"os"
	"regexp"
	"syscall"

	"github.com/UrbanCompass/hnvm"
	"github.com/UrbanCompass/hnvm/internal/config"
	"github.com/UrbanCompass/hnvm/internal/install"
	"github.com/UrbanCompass/hnvm/internal/output"
)

var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func loadConfig() (*config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	return config.Load(config.Inputs{Env: os.LookupEnv, Cwd: cwd, Home: home, DefaultRC: hnvm.DefaultRC})
}

// Node runs `node` at the version the configuration selects and only returns on failure,
// with the process exit code to use.
func Node(args []string) int {
	cfg, err := loadConfig()
	if err != nil {
		output.New("", false).Error("%v", err)
		return 1
	}
	out := output.New(os.Getenv("HNVM_OUTPUT_DESTINATION"), cfg.Bool("HNVM_QUIET"))

	version := cfg.Get("HNVM_NODE")
	if version == "" {
		out.Error("No HNVM_NODE version set. Please set a Node version.")
		return 1
	}
	if !exactVersion.MatchString(version) {
		// Tags and ranges need the registry client and semver package (Phase 2).
		out.Error("Go shim: resolving %q is not supported yet, pin an exact version", version)
		return 1
	}

	n := &install.Node{
		Version:  version,
		Root:     cfg.Get("HNVM_PATH"),
		Dist:     cfg.Get("HNVM_NODE_DIST"),
		Variant:  cfg.Get("HNVM_NODE_VARIANT"),
		SkipHead: cfg.Bool("HNVM_SKIP_URL_VALIDATION"),
		Out:      out,
	}
	if err := n.Ensure(context.Background()); err != nil {
		if err != install.ErrReported {
			out.Error("%v", err)
		}
		return 1
	}

	out.Info("Using Hermetic NodeJS v%s", version)

	argv := append([]string{n.Bin()}, args...)
	err = syscall.Exec(n.Bin(), argv, os.Environ())
	out.Error("exec %s: %v", n.Bin(), err)
	return 126
}
