package distrust

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

var (
	pemBegin = []byte("-----BEGIN CERTIFICATE-----")
	pemEnd   = []byte("-----END CERTIFICATE-----")
)

// patchBinary overwrites, in place in bin, every PEM certificate whose SHA-256 is in distrusted with placeholder, padded
// with '\n' to the length of the original block. src is only used in error messages. It returns how many blocks were
// removed and how many PEM certificate blocks the binary has in total. It fails when the placeholder is longer than a
// block or when the binary has no PEM blocks at all.
func patchBinary(bin []byte, src string, distrusted map[string]bool, placeholder []byte) (removed, blocks int, err error) {
	// Matches the JS port: a block runs from a BEGIN marker to the first END marker after it, and the next search starts after the END.
	start := bytes.Index(bin, pemBegin)
	for start != -1 {
		rel := bytes.Index(bin[start:], pemEnd)
		if rel == -1 {
			break
		}
		stop := start + rel + len(pemEnd)
		blocks++

		if distrusted[pemFingerprint(bin[start:stop])] {
			length := stop - start
			if len(placeholder) > length {
				return removed, blocks, fmt.Errorf("Placeholder certificate (%d bytes) is larger than a %d byte root", len(placeholder), length)
			}
			region := bin[start:stop]
			n := copy(region, placeholder)
			for i := n; i < length; i++ {
				region[i] = '\n'
			}
			removed++
		}

		next := bytes.Index(bin[stop:], pemBegin)
		if next == -1 {
			break
		}
		start = stop + next
	}

	if blocks == 0 {
		return removed, blocks, fmt.Errorf("Could not find any bundled root certificates in %s", src)
	}
	return removed, blocks, nil
}

// pemFingerprint returns the SHA-256 (hex) of the DER certificate inside one PEM block, the same as the JS port: whitespace is
// dropped from the body and the rest is decoded leniently as base64.
func pemFingerprint(block []byte) string {
	body := block[len(pemBegin) : len(block)-len(pemEnd)]
	sum := sha256.Sum256(decodeBase64Lenient(body))
	return hex.EncodeToString(sum[:])
}

// decodeBase64Lenient decodes base64 the way Node's Buffer.from(str, 'base64') does: characters outside the alphabet (including
// whitespace) are skipped, and decoding stops at the first '='. Accepts the URL-safe alphabet too.
func decodeBase64Lenient(s []byte) []byte {
	var out []byte
	var acc uint32
	bits := 0
	for _, c := range s {
		if c == '=' {
			break
		}
		v := base64Value(c)
		if v < 0 {
			continue
		}
		acc = acc<<6 | uint32(v)
		bits += 6
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>bits))
			acc &= (1 << bits) - 1
		}
	}
	return out
}

func base64Value(c byte) int {
	switch {
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 26
	case c >= '0' && c <= '9':
		return int(c-'0') + 52
	case c == '+' || c == '-':
		return 62
	case c == '/' || c == '_':
		return 63
	}
	return -1
}
