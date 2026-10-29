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
	// security notices use the non-bold yellow that min-safe-node.sh used
	yellowPlain = "\x1b[33m"
	reset       = "\x1b[0m"
)

// Output is the destination for informational messages ("Using Hermetic NodeJS ...").
// Errors and warnings always go to stderr regardless of it.
type Output struct {
	info     io.Writer
	err      io.Writer
	security func(string)
}

// New honors HNVM_OUTPUT_DESTINATION (a socket is replaced by /dev/null, with a warning) and
// HNVM_QUIET. Informational output defaults to stderr.
func New(destination string, quiet bool) *Output {
	o := &Output{info: os.Stderr, err: os.Stderr}
	o.security = func(msg string) { fmt.Fprintf(o.err, "%sWarning%s: %s\n", yellowPlain, reset, msg) }

	if destination != "" {
		isSocket := false
		if fi, err := os.Stat(destination); err == nil && fi.Mode()&os.ModeSocket != 0 {
			isSocket = true
		}
		if !isSocket {
			o.security = func(msg string) {
				if f, err := os.OpenFile(destination, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644); err == nil {
					fmt.Fprintf(f, "%sWarning%s: %s\n", yellowPlain, reset, msg)
					f.Close()
				}
			}
		}
		if isSocket {
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

// Security writes a security notice ("Warning: ..."). It is not hidden by HNVM_QUIET, but it does
// follow HNVM_OUTPUT_DESTINATION when that is a writable non-socket. A failed write is ignored, so a
// notice can never stop the tool from running.
func (o *Output) Security(format string, args ...any) {
	o.security(fmt.Sprintf(format, args...))
}

// NewWriter sends informational output to w (errors and security notices still go to stderr).
func NewWriter(w io.Writer, quiet bool) *Output {
	o := New("", quiet)
	if !quiet {
		o.info = w
	}
	return o
}

// Discard is an Output that prints nothing at all.
func Discard() *Output {
	return &Output{info: io.Discard, err: io.Discard, security: func(string) {}}
}

// Caution writes "Warning: ..." to the command output, so HNVM_QUIET hides it (unlike Security).
func (o *Output) Caution(format string, args ...any) {
	fmt.Fprintf(o.info, "%sWarning%s: %s\n", yellowPlain, reset, fmt.Sprintf(format, args...))
}

// Plain writes an uncolored line to the command output.
func (o *Output) Plain(format string, args ...any) {
	fmt.Fprintf(o.info, format+"\n", args...)
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
