// Package hnvm only carries the data files that ship with HNVM, compiled into the binary so a
// release is one self-contained file.
package hnvm

import _ "embed"

// DefaultRC is the .hnvmrc that ships with HNVM. It is the last (lowest priority) rc file consulted.
//
//go:embed .hnvmrc
var DefaultRC string
