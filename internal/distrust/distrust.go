// Package distrust removes distrusted root CA certificates from the Node.js binaries HNVM installs. It is the Go port of
// the distrust step in lib/hnvm/ensure_bin.sh. The marker file, lock directory and messages match the bash version
// byte-for-byte, so bash and Go HNVM can share ~/.hnvm.
package distrust

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/UrbanCompass/hnvm/internal/output"
)

// ErrRefused is returned by Apply when strict mode says node must not run. The reason has already been printed.
var ErrRefused = errors.New("node will not run: distrusted root certificates could not be removed")

// Settings mirrors the HNVM_DISTRUST_CERTS* variables and the MDM flags that decide the message wording.
type Settings struct {
	// Enabled is HNVM_DISTRUST_CERTS != "false". Apply does not check it; callers skip Apply when it is false.
	Enabled bool
	// Extra is HNVM_DISTRUST_CERTS_EXTRA: comma-separated fingerprints, optionally in openssl "sha256 Fingerprint=AA:BB" form.
	Extra string
	// Strict is HNVM_DISTRUST_CERTS_STRICT == "true". A failure then refuses to run node instead of warning.
	Strict bool

	// EnabledManaged, ExtraManaged and StrictManaged report that an MDM profile set the matching setting.
	EnabledManaged bool
	ExtraManaged   bool
	StrictManaged  bool
}

const (
	markerVersion  = "v1"
	markerFileName = ".hnvm-distrusted-certs"
	lockDirName    = ".hnvm-distrust-lock"
	lockStaleAfter = 2 * time.Minute
	lockRetryDelay = 200 * time.Millisecond
	maxLockRetries = 20
)

var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Apply makes sure the node install at nodeDir (binary at nodeDir/bin/node, version for messages) has the distrusted
// roots in list (the contents of distrusted-certs.txt) and s.Extra neutralized, and records the result in a marker file.
//
// It returns ErrRefused only when strict mode requires node not to run. In every other case the problem is reported as a
// warning and nil is returned. placeholder is the expired certificate written over each removed root.
func Apply(nodeDir, version string, s Settings, list, placeholder string, out *output.Output) error {
	a := &applier{
		nodeDir:     nodeDir,
		nodeBin:     filepath.Join(nodeDir, "bin", "node"),
		version:     version,
		s:           s,
		placeholder: placeholder,
		out:         out,
	}
	return a.apply(list)
}

type applier struct {
	nodeDir     string
	nodeBin     string
	version     string
	s           Settings
	placeholder string
	out         *output.Output
}

func (a *applier) apply(list string) error {
	// Same as $(< distrusted-certs.txt): trailing newlines are dropped.
	certs := strings.TrimRight(list, "\n")
	marker := markerVersion + "\n" + certs + "\nextra=" + a.s.Extra
	markerFile := filepath.Join(a.nodeDir, markerFileName)

	// Fast path, run on every invocation: patched already, and nothing to say about invalid entries.
	if recorded, ok := readMarker(markerFile); ok {
		if recorded == marker {
			return nil
		}
		// Patched already, apart from invalid entries that are still listed: repeat the warning without patching.
		if invalid, found := strings.CutPrefix(recorded, marker+"\ninvalid="); found {
			return a.invalidFailed(invalid)
		}
	}

	var valid, invalid []string
	for _, fp := range normalizeFingerprints(certs, a.s.Extra) {
		if fingerprintPattern.MatchString(fp) {
			valid = append(valid, fp)
		} else {
			invalid = append(invalid, fp)
		}
	}
	invalidMarker := ""
	if len(invalid) > 0 {
		if err := a.invalidFailed(strings.Join(invalid, ",")); err != nil {
			return err
		}
		// Still remove the valid ones, so a bad entry can't be used to skip the rest. The marker records the invalid
		// entries so later runs keep warning until the list is fixed.
		invalidMarker = "\ninvalid=" + strings.Join(invalid, ",")
	}

	lockDir := filepath.Join(a.nodeDir, lockDirName)
	if !a.acquireLock(lockDir) {
		return a.fail("could not create " + lockDir)
	}
	defer func() { _ = os.Remove(lockDir) }()

	want := marker + invalidMarker
	if recorded, ok := readMarker(markerFile); !ok || recorded != want {
		return a.patch(valid, want)
	}
	return nil
}

// acquireLock takes the mkdir lock that bash ensure_bin.sh also uses, so the two interlock. A lock older than two minutes
// was left behind by a killed process and is removed. It returns false when the lock can't be created at all.
func (a *applier) acquireLock(lockDir string) bool {
	retries := 0
	for {
		if err := os.Mkdir(lockDir, 0o777); err == nil {
			return true
		}
		fi, err := os.Stat(lockDir)
		if err != nil || !fi.IsDir() {
			// Usually the lock was just released, so try again right away. Only give up if it can't be created.
			if !writable(a.nodeDir) {
				return false
			}
			retries++
			if retries > maxLockRetries {
				return false
			}
			continue
		}
		if time.Since(fi.ModTime()) > lockStaleAfter {
			_ = os.Remove(lockDir)
		}
		time.Sleep(lockRetryDelay)
	}
}

// patch writes the patched binary to a temporary file next to node, verifies it and renames it over node. Called with
// the lock held. markerContent is written when this completes, including when nothing matched.
func (a *applier) patch(fps []string, markerContent string) error {
	tmp, err := os.CreateTemp(filepath.Dir(a.nodeBin), filepath.Base(a.nodeBin)+".hnvm-tmp.*")
	if err != nil {
		return a.fail(fmt.Sprintf("could not create a temporary file next to %s: %v", a.nodeBin, err))
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	replaced := false
	defer func() {
		if !replaced {
			_ = os.Remove(tmpPath)
		}
	}()

	src, err := os.ReadFile(a.nodeBin)
	if err != nil {
		return a.fail(fmt.Sprintf("could not read %s: %v", a.nodeBin, err))
	}
	fi, err := os.Stat(a.nodeBin)
	if err != nil {
		return a.fail(fmt.Sprintf("could not stat %s: %v", a.nodeBin, err))
	}

	set := make(map[string]bool, len(fps))
	for _, fp := range fps {
		set[fp] = true
	}
	patched := bytes.Clone(src)
	removed, blocks, err := patchBinary(patched, a.nodeBin, set, []byte(strings.TrimSpace(a.placeholder)))
	if err != nil {
		return a.fail(err.Error())
	}
	if removed == 0 {
		return a.writeMarker(markerContent)
	}

	roots, err := a.rootCount()
	if err != nil {
		return a.fail(err.Error())
	}
	if err := os.WriteFile(tmpPath, patched, 0o600); err != nil {
		return a.fail(fmt.Sprintf("could not write %s: %v", tmpPath, err))
	}
	if err := os.Chmod(tmpPath, fi.Mode()); err != nil {
		return a.fail(fmt.Sprintf("could not chmod %s: %v", tmpPath, err))
	}

	if runtime.GOOS == "darwin" {
		if out, err := execCommand("codesign", "--force", "--sign", "-", "--preserve-metadata=entitlements", tmpPath); err != nil {
			return a.fail("re-signing failed: " + trimOutput(out))
		}
	}

	if out, err := execCommand(tmpPath, "-e", verifyScript(fps, blocks, roots)); err != nil {
		return a.fail("the patched binary failed verification: " + trimOutput(out))
	}

	if err := os.Rename(tmpPath, a.nodeBin); err != nil {
		return a.fail(fmt.Sprintf("could not replace %s: %v", a.nodeBin, err))
	}
	replaced = true

	a.out.Info("Removed %d distrusted root certificate(s) from node v%s", removed, a.version)
	return a.writeMarker(markerContent)
}

// fail reports a failure to remove the distrusted roots. In strict mode it prints an error and returns ErrRefused,
// otherwise it prints a warning and returns nil so node still runs.
func (a *applier) fail(reason string) error {
	if a.s.Strict {
		a.out.Error("Unable to remove distrusted root certificates from node v%s: %s", a.version, reason)
		if a.s.EnabledManaged || a.s.StrictManaged {
			a.out.Error("Your organization requires this, so node will not run until they can be removed.")
		} else {
			a.out.Error("Set HNVM_DISTRUST_CERTS=false to run this node anyway (it will trust those certificates).")
		}
		return ErrRefused
	}
	a.out.Warning("node v%s still trusts distrusted root certificates: %s", a.version, reason)
	return nil
}

func (a *applier) invalidFailed(invalid string) error {
	if a.s.ExtraManaged {
		return a.fail("invalid SHA-256 fingerprint(s) in HNVM_DISTRUST_CERTS_EXTRA (yours or your organization's): " + invalid)
	}
	return a.fail("invalid SHA-256 fingerprint(s) in HNVM_DISTRUST_CERTS_EXTRA: " + invalid)
}

// writeMarker records the state. Errors are ignored, like the bash version: a missing marker only costs a re-check next run.
func (a *applier) writeMarker(content string) error {
	_ = os.WriteFile(filepath.Join(a.nodeDir, markerFileName), []byte(content+"\n"), 0o644)
	return nil
}

// readMarker returns the marker file content with trailing newlines removed, as $(< file) does.
func readMarker(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(data), "\n"), true
}

// normalizeFingerprints turns the distrust list and HNVM_DISTRUST_CERTS_EXTRA into the fingerprints to remove, the same way
// ensure_bin.sh does. Blank and comment lines of the list are skipped. Everything up to the last '=' (an openssl
// "sha256 Fingerprint=" label) is dropped, colons and whitespace are removed, and the result is lowercased, deduplicated
// and sorted. Invalid entries are returned too; the caller checks them.
func normalizeFingerprints(list, extra string) []string {
	var entries []string
	for _, line := range strings.Split(list, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		entries = append(entries, line)
	}
	entries = append(entries, strings.Split(extra, ",")...)

	seen := make(map[string]bool)
	var fps []string
	for _, entry := range entries {
		if i := strings.LastIndex(entry, "="); i != -1 {
			entry = entry[i+1:]
		}
		fp := strings.Map(func(r rune) rune {
			if r == ':' || unicode.IsSpace(r) {
				return -1
			}
			return unicode.ToLower(r)
		}, entry)
		if fp == "" || seen[fp] {
			continue
		}
		seen[fp] = true
		fps = append(fps, fp)
	}
	sort.Strings(fps)
	return fps
}

// writable reports whether the current user may create entries in dir, like bash's -w test. It probes by creating a file,
// since the stdlib has no portable access(2) wrapper.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".hnvm-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
