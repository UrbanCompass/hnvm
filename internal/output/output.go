// Package output decides where HNVM's own messages go. They must never land on stdout, which
// belongs to the tool being run.
package output

import (
	"fmt"
	"io"
	"os"
)

const (
	blue   = "\x1b[1;34m"
	red    = "\x1b[1;31m"
	yellow = "\x1b[1;33m"
	reset  = "\x1b[0m"
)

// Output is the destination for informational messages ("Using Hermetic NodeJS ...").
// Errors and warnings always go to stderr regardless of it.
type Output struct {
	info io.Writer
	err  io.Writer
}

// New honors HNVM_OUTPUT_DESTINATION (a socket is replaced by /dev/null, with a warning) and
// HNVM_QUIET. Informational output defaults to stderr.
func New(destination string, quiet bool) *Output {
	o := &Output{info: os.Stderr, err: os.Stderr}

	if destination != "" {
		if fi, err := os.Stat(destination); err == nil && fi.Mode()&os.ModeSocket != 0 {
			fmt.Fprintln(o.err, "WARNING: Could not find a writable, non-socket stdout redirect target!")
			fmt.Fprintln(o.err, "WARNING: Further HNVM output will be redirected to '/dev/null'")
			o.info = io.Discard
		} else if f, err := os.OpenFile(destination, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644); err == nil {
			o.info = f
		} else {
			o.info = io.Discard
		}
	}

	if quiet {
		o.info = io.Discard
	}
	return o
}

// Info writes a blue informational line to the command output.
func (o *Output) Info(format string, args ...any) {
	fmt.Fprintf(o.info, "%s%s%s\n", blue, fmt.Sprintf(format, args...), reset)
}

// Error writes to stderr, unaffected by HNVM_QUIET.
func (o *Output) Error(format string, args ...any) {
	fmt.Fprintf(o.err, "%sERROR%s: %s\n", red, reset, fmt.Sprintf(format, args...))
}

// Warning writes to stderr, unaffected by HNVM_QUIET.
func (o *Output) Warning(format string, args ...any) {
	fmt.Fprintf(o.err, "%sWARNING%s: %s\n", yellow, reset, fmt.Sprintf(format, args...))
}
