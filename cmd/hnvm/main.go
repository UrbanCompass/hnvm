// Command hnvm is a busybox-style binary: invoked as node/npm/pnpm/... (through symlinks or the
// bootstrap shims) it acts as that tool's shim, invoked as hnvm it is the management CLI.
package main

import (
	"os"
	"path/filepath"

	"github.com/UrbanCompass/hnvm/internal/cli"
	"github.com/UrbanCompass/hnvm/internal/shim"
)

// version is set at release time with -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if name := filepath.Base(os.Args[0]); shim.IsTool(name) {
		os.Exit(shim.Run(name, os.Args[1:]))
	}
	os.Exit(cli.Main(version, os.Args[1:]))
}
