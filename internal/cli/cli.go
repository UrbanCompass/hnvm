// Package cli is the hnvm management command. The shims (node, npm, ...) are invisible; this is the
// part people run on purpose, to see which version applies and why, warm caches, and clean up.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"

	"github.com/UrbanCompass/hnvm/internal/output"
	"github.com/UrbanCompass/hnvm/internal/shim"
)

// CLI is the hnvm command tree.
type CLI struct {
	Current   CurrentCmd   `kong:"cmd,help='Show the version of each tool for this directory, and which setting decided it.'"`
	Which     WhichCmd     `kong:"cmd,help='Print the path of the binary a tool would run.'"`
	Config    ConfigCmd    `kong:"cmd,help='Show the effective configuration and where each value comes from.'"`
	Install   InstallCmd   `kong:"cmd,help='Download tools ahead of time (for example to warm a CI cache).'"`
	List      ListCmd      `kong:"cmd,aliases='ls',help='List installed versions.'"`
	Uninstall UninstallCmd `kong:"cmd,aliases='rm',help='Remove installed versions.'"`
	Prune     PruneCmd     `kong:"cmd,help='Remove installed versions that have not been used recently.'"`
	Exec      ExecCmd      `kong:"cmd,help='Run a tool without relying on the shims being on PATH.'"`
	Doctor    DoctorCmd    `kong:"cmd,help='Check the hnvm installation and environment for problems.'"`
	Shims     ShimsCmd     `kong:"cmd,help='Create symlinks for every tool hnvm manages (node, npm, pnpm, ...).'"`
	Version   VersionCmd   `kong:"cmd,help='Print the hnvm version.'"`
}

// Globals are bound into every command's Run.
type Globals struct {
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
}

// Main runs the hnvm command and returns the exit code.
func Main(version string, args []string) int {
	return run(version, args, os.Stdout, os.Stderr)
}

func run(version string, args []string, stdout, stderr io.Writer) int {
	var cli CLI
	exit := -1
	parser, err := kong.New(&cli,
		kong.Name("hnvm"),
		kong.Description("Hermetic Node Version Manager: hermetic node, npm, pnpm, yarn and bun versions per project."),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { exit = code; panic(exitPanic{}) }),
		kong.Bind(&Globals{Version: version, Stdout: stdout, Stderr: stderr}),
	)
	if err != nil {
		fmt.Fprintf(stderr, "hnvm: %v\n", err)
		return 1
	}

	return parse(parser, args, &exit)
}

type exitPanic struct{}

// parse runs the selected command. kong calls Exit for --help and usage errors, which we turn
// into a return value so tests can drive the CLI in-process.
func parse(parser *kong.Kong, args []string, exit *int) (code int) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(exitPanic); !ok {
				panic(r)
			}
			code = *exit
		}
	}()
	if len(args) == 0 {
		args = []string{"--help"}
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		parser.Errorf("%s", err)
		return 2
	}
	if err := ctx.Run(); err != nil {
		var ec exitCode
		if errors.As(err, &ec) {
			return int(ec)
		}
		if !errors.Is(err, shim.ErrReported) {
			fmt.Fprintf(parser.Stderr, "hnvm: %v\n", err)
		}
		return 1
	}
	return 0
}

// exitCode lets a command fail with a specific code without printing anything more.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// session loads the configuration for the current directory. HNVM's messages go to stderr.
func session(g *Globals) (*shim.Session, error) {
	return shim.NewSession(output.NewWriter(g.Stderr, false))
}
