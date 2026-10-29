// Command hnvm is a busybox-style binary: invoked as node/npm/pnpm/... (through symlinks or the
// bootstrap shims) it acts as that tool's shim, invoked as hnvm it is the management CLI.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/UrbanCompass/hnvm/internal/shim"
)

func main() {
	name := filepath.Base(os.Args[0])
	switch name {
	case "node":
		os.Exit(shim.Node(os.Args[1:]))
	default:
		// The management CLI (internal/cli) arrives in Phase 4.
		fmt.Fprintf(os.Stderr, "hnvm: %q is not handled by the Go binary yet\n", name)
		os.Exit(1)
	}
}
