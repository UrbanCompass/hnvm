package shim

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/UrbanCompass/hnvm/internal/install"
)

// Tools lists every name the binary answers to as a shim.
var Tools = []string{"node", "npm", "npx", "pnpm", "pnpx", "yarn", "bun", "bunx"}

// IsTool reports whether name is one of Tools.
func IsTool(name string) bool {
	for _, t := range Tools {
		if t == name {
			return true
		}
	}
	return false
}

// Run acts as the shim for tool and only returns on failure, with the exit code to use.
func Run(tool string, args []string) int {
	s, err := NewSession(nil)
	if err != nil {
		return fail(nil, err)
	}
	argv, err := s.Command(context.Background(), tool, args)
	if err != nil {
		return fail(s, err)
	}

	if isKondo(tool, args) {
		return kondo(s, argv)
	}
	return Exec(s, argv)
}

// Command prepares tool and returns the full command line to run it with args.
func (s *Session) Command(ctx context.Context, tool string, args []string) ([]string, error) {
	switch tool {
	case "bun", "bunx":
		// bun is a standalone binary and never needs node
		version, err := s.Tool(ctx, "bun")
		if err != nil {
			return nil, err
		}
		s.Out.Info("Using Hermetic bun v%s", version)
		bin := filepath.Join(s.Root(), "bun", version, "bin", "bun")
		if tool == "bunx" {
			return append([]string{bin, "x"}, args...), nil
		}
		return append([]string{bin}, args...), nil
	}

	version, dir, err := s.Node(ctx)
	if err != nil {
		return nil, err
	}
	node := filepath.Join(dir, "bin", "node")

	var script string
	switch tool {
	case "node":
	case "npm", "npx":
		script = filepath.Join(dir, "bin", tool)
	case "pnpm", "pnpx":
		v, err := s.Tool(ctx, "pnpm")
		if err != nil {
			return nil, err
		}
		script = install.PnpmBin(s.Root(), v, tool)
		// Keep the node that runs pnpm first on PATH for scripts, and run pre/post scripts in pnpm 6+
		os.Setenv("npm_config_scripts_prepend_node_path", "true")
		os.Setenv("npm_config_enable_pre_post_scripts", "true")
	case "yarn":
		v, err := s.Tool(ctx, "yarn")
		if err != nil {
			return nil, err
		}
		script = filepath.Join(s.Root(), "yarn", v, "bin", "yarn.js")
	default:
		return nil, fmt.Errorf("%s is not a tool hnvm manages", tool)
	}

	s.Out.Info("Using Hermetic NodeJS v%s", version)
	if script == "" {
		return append([]string{node}, args...), nil
	}
	return append([]string{node, script}, args...), nil
}

// Exec replaces the process with argv. It only returns on failure.
func Exec(s *Session, argv []string) int {
	err := syscall.Exec(argv[0], argv, os.Environ())
	s.Out.Error("exec %s: %v", argv[0], err)
	return 126
}

func fail(s *Session, err error) int {
	if err != ErrReported {
		if s != nil {
			s.Out.Error("%v", err)
		} else {
			fmt.Fprintf(os.Stderr, "hnvm: %v\n", err)
		}
	}
	return 1
}
