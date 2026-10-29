// Package hnvm only carries the data files that ship with HNVM, compiled into the binary so a
// release is one self-contained file.
package hnvm

import _ "embed"

// DefaultRC is the .hnvmrc that ships with HNVM. It is the last (lowest priority) rc file consulted.
//
//go:embed .hnvmrc
var DefaultRC string

// MinSafeNode holds the minimum safe Node release rules (see the file for the format).
//
//go:embed lib/hnvm/min-safe-node.txt
var MinSafeNode string

// DistrustedCerts lists the SHA-256 fingerprints of root certificates removed from managed Node binaries.
//
//go:embed lib/hnvm/distrusted-certs.txt
var DistrustedCerts string

// DistrustedPlaceholder is the expired certificate written over each distrusted root.
//
//go:embed lib/hnvm/distrusted-placeholder.pem
var DistrustedPlaceholder string
