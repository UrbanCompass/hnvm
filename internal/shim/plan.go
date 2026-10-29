// Package shim prepares and runs the tools HNVM manages: it resolves versions, installs what is
// missing, applies the security policy and finally replaces itself with the tool.
package shim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/UrbanCompass/hnvm"
	"github.com/UrbanCompass/hnvm/internal/config"
	"github.com/UrbanCompass/hnvm/internal/distrust"
	"github.com/UrbanCompass/hnvm/internal/install"
	"github.com/UrbanCompass/hnvm/internal/managed"
	"github.com/UrbanCompass/hnvm/internal/minsafe"
	"github.com/UrbanCompass/hnvm/internal/output"
	"github.com/UrbanCompass/hnvm/internal/resolve"
)

// ErrReported means the failure was already printed and the caller should only exit non-zero.
var ErrReported = errors.New("")

// Session is the configuration, output and policy for one invocation.
type Session struct {
	Cfg     *config.Config
	Out     *output.Output
	Managed *managed.Prefs
	Res     *resolve.Resolver
	Inst    *install.Env
}

// NewSession loads the configuration for the current directory. A broken package.json is reported
// here, exactly like bash.
func NewSession(out *output.Output) (*Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	cfg, err := config.Load(config.Inputs{Env: os.LookupEnv, Cwd: cwd, Home: home, DefaultRC: hnvm.DefaultRC})
	if out == nil {
		quiet := cfg != nil && cfg.Bool("HNVM_QUIET")
		out = output.New(os.Getenv("HNVM_OUTPUT_DESTINATION"), quiet)
	}
	if err != nil {
		out.Error("%v", err)
		return nil, ErrReported
	}

	root := cfg.Get("HNVM_PATH")
	ttl, _ := strconv.Atoi(cfg.Get("HNVM_RANGE_CACHE"))
	return &Session{
		Cfg:     cfg,
		Out:     out,
		Managed: managed.LoadDefault(),
		Res:     &resolve.Resolver{Root: root, CacheTTL: time.Duration(ttl) * time.Second, Out: out},
		Inst:    &install.Env{Root: root, SkipHead: cfg.Bool("HNVM_SKIP_URL_VALIDATION"), Out: out},
	}, nil
}

// SetOutput redirects every message of the session, including the resolver's and installers'.
func (s *Session) SetOutput(out *output.Output) {
	s.Out, s.Res.Out, s.Inst.Out = out, out, out
}

// Root is $HNVM_PATH.
func (s *Session) Root() string { return s.Cfg.Get("HNVM_PATH") }

// NodeVersion resolves HNVM_NODE to an exact version and applies the minimum safe version policy.
// pinned is the exact version before that policy, for `hnvm current`.
func (s *Session) NodeVersion(ctx context.Context) (version string, notice string, err error) {
	spec := s.Cfg.Get("HNVM_NODE")
	if spec == "" {
		s.Out.Error("No HNVM_NODE version set. Please set a Node version.")
		return "", "", ErrReported
	}
	res, err := s.Res.Resolve(ctx, "node", spec)
	if err != nil {
		return "", "", reported(err)
	}
	r := s.MinSafe(res.Version)
	if r.Notice != "" {
		s.Out.Security("%s", r.Notice)
	}
	return r.Version, r.Notice, nil
}

// MinSafe applies the minimum safe version policy to an exact node version, without printing.
func (s *Session) MinSafe(version string) minsafe.Result {
	return minsafe.Apply(version, minsafe.ParseRules(hnvm.MinSafeNode), minsafe.Settings{
		SameLine:        s.Cfg.Get("HNVM_MIN_SAFE_NODE"),
		CrossMajor:      s.Cfg.Get("HNVM_MIN_SAFE_NODE_MAJOR"),
		ManagedSameLine: func() (bool, bool) { return s.Managed.Bool("HNVM_MIN_SAFE_NODE") },
		ManagedMajor:    func() (bool, bool) { return s.Managed.Bool("HNVM_MIN_SAFE_NODE_MAJOR") },
	})
}

// Node resolves, installs and secures node, returning its install directory.
func (s *Session) Node(ctx context.Context) (version, dir string, err error) {
	version, _, err = s.NodeVersion(ctx)
	if err != nil {
		return "", "", err
	}
	n := &install.Node{
		Version:  version,
		Root:     s.Root(),
		Dist:     s.Cfg.Get("HNVM_NODE_DIST"),
		Variant:  s.Cfg.Get("HNVM_NODE_VARIANT"),
		SkipHead: s.Cfg.Bool("HNVM_SKIP_URL_VALIDATION"),
		Out:      s.Out,
	}
	if err := n.Ensure(ctx); err != nil {
		return "", "", s.report(err)
	}

	// Something sometimes installs pnpm globally into node's bin directory, where it would shadow hnvm's
	// pnpm in npm scripts.
	for _, name := range []string{"pnpm", "pnpx"} {
		if p := filepath.Join(n.Dir(), "bin", name); fileExists(p) {
			s.Out.Warning("Found conflicting global install of %s, removing...", name)
			os.Remove(p)
		}
	}

	ds := s.Managed.ApplyDistrust(s.Cfg.Get("HNVM_DISTRUST_CERTS"), s.Cfg.Get("HNVM_DISTRUST_CERTS_EXTRA"), s.Cfg.Get("HNVM_DISTRUST_CERTS_STRICT"))
	if ds.Enabled != "false" {
		err := distrust.Apply(n.Dir(), version, distrust.Settings{
			Enabled:        true,
			Extra:          ds.Extra,
			Strict:         ds.Strict == "true",
			EnabledManaged: ds.EnabledManaged,
			ExtraManaged:   ds.ExtraManaged,
			StrictManaged:  ds.StrictManaged,
		}, hnvm.DistrustedCerts, hnvm.DistrustedPlaceholder, s.Out)
		if err != nil {
			return "", "", ErrReported
		}
	}
	return version, n.Dir(), nil
}

// Tool resolves and installs pnpm, yarn or bun, returning the exact version.
func (s *Session) Tool(ctx context.Context, tool string) (string, error) {
	key := "HNVM_" + map[string]string{"pnpm": "PNPM", "yarn": "YARN", "bun": "BUN"}[tool]
	spec := s.Cfg.Get(key)
	if spec == "" {
		s.Out.Error("No %s version set. Please set a %s version.", key, tool)
		return "", ErrReported
	}
	res, err := s.Res.Resolve(ctx, tool, spec)
	if err != nil {
		return "", reported(err)
	}
	switch tool {
	case "pnpm":
		err = s.Inst.EnsurePnpm(ctx, res.Version, s.Cfg.Get("HNVM_PNPM_REGISTRY"))
	case "yarn":
		err = s.Inst.EnsureYarn(ctx, res.Version, s.Cfg.Get("HNVM_YARN_DIST"))
	case "bun":
		err = s.Inst.EnsureBun(ctx, res.Version, s.Cfg.Get("HNVM_BUN_DIST"), s.Cfg.Get("HNVM_BUN_VARIANT"))
	}
	if err != nil {
		return "", s.report(err)
	}
	return res.Version, nil
}

func (s *Session) report(err error) error {
	if err != install.ErrReported {
		s.Out.Error("%v", err)
	}
	return ErrReported
}

func reported(err error) error {
	if err == resolve.ErrReported {
		return ErrReported
	}
	return err
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
