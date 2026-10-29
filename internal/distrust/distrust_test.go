package distrust

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/UrbanCompass/hnvm"
	"github.com/UrbanCompass/hnvm/internal/output"
)

const testVersion = "18.12.0"

// testCert is a synthetic self-signed root with its SHA-256 fingerprint (of the DER) and PEM block.
type testCert struct {
	block []byte
	fp    string
}

// newTestCert makes a self-signed CA certificate whose PEM block size grows with len(cn).
func newTestCert(t *testing.T, cn string) testCert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return testCert{
		block: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		fp:    hex.EncodeToString(sum[:]),
	}
}

// blockFingerprints lists the fingerprint of every PEM block in bin, using the same scan as patchBinary.
func blockFingerprints(bin []byte) []string {
	var fps []string
	start := bytes.Index(bin, pemBegin)
	for start != -1 {
		rel := bytes.Index(bin[start:], pemEnd)
		if rel == -1 {
			break
		}
		stop := start + rel + len(pemEnd)
		fps = append(fps, pemFingerprint(bin[start:stop]))
		next := bytes.Index(bin[stop:], pemBegin)
		if next == -1 {
			break
		}
		start = stop + next
	}
	return fps
}

// fakeNode stands in for the node binary: it answers the root count and verify scripts without running node.
type fakeNode struct {
	mu          sync.Mutex
	distrusted  map[string]bool
	roots       int
	rootsOutput *string // when set, the root count step prints this instead
	verifyErr   error
	rootCalls   int
	verifyCalls int
}

// calls returns the verify and root-count call counts, read under the lock because Apply may still be running elsewhere.
func (f *fakeNode) calls() (verify, roots int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verifyCalls, f.rootCalls
}

func installFakeNode(t *testing.T, f *fakeNode) {
	t.Helper()
	prev := execCommand
	execCommand = func(name string, args ...string) ([]byte, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(args) != 2 || args[0] != "-e" {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		switch {
		case strings.Contains(args[1], "hnvm-distrust-roots"):
			f.rootCalls++
			if f.rootsOutput != nil {
				return []byte(*f.rootsOutput), nil
			}
			return []byte(fmt.Sprintf("hnvm-distrust-roots %d\n", f.roots)), nil
		case strings.Contains(args[1], "distrustedFound"):
			f.verifyCalls++
			if f.verifyErr != nil {
				return []byte("verify exploded\n"), f.verifyErr
			}
			data, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			for _, fp := range blockFingerprints(data) {
				if f.distrusted[fp] {
					return []byte("still there\n"), errors.New("exit status 1")
				}
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected script for %s", name)
	}
	t.Cleanup(func() { execCommand = prev })
}

type fixture struct {
	nodeDir     string
	binDir      string
	nodeBin     string
	markerFile  string
	lockDir     string
	orig        []byte
	placeholder string
	fake        *fakeNode
	certs       []testCert
}

// newFixture writes a synthetic node binary holding certs (junk between them) and installs a fake node that treats removeFPs
// as distrusted.
func newFixture(t *testing.T, certs []testCert, removeFPs ...string) *fixture {
	t.Helper()
	root := t.TempDir()
	nodeDir := filepath.Join(root, "node", testVersion)
	binDir := filepath.Join(nodeDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var blob bytes.Buffer
	junk := []byte("\x7fELF\x00\xff junk bytes between certificates \x00\x01")
	blob.Write(junk)
	for _, c := range certs {
		blob.Write(c.block)
		blob.Write(junk)
	}
	nodeBin := filepath.Join(binDir, "node")
	if err := os.WriteFile(nodeBin, blob.Bytes(), 0o755); err != nil {
		t.Fatal(err)
	}

	distrusted := make(map[string]bool)
	for _, fp := range removeFPs {
		distrusted[fp] = true
	}
	fake := &fakeNode{distrusted: distrusted, roots: 150}
	installFakeNode(t, fake)

	return &fixture{
		nodeDir:     nodeDir,
		binDir:      binDir,
		nodeBin:     nodeBin,
		markerFile:  filepath.Join(nodeDir, markerFileName),
		lockDir:     filepath.Join(nodeDir, lockDirName),
		orig:        blob.Bytes(),
		placeholder: string(newTestCert(t, "placeholder").block) + "\n",
		fake:        fake,
		certs:       certs,
	}
}

// listOf builds a distrusted-certs.txt style list: a comment, the fingerprints, and trailing blank lines (which are trimmed).
func listOf(fps ...string) string {
	return "# test list\n" + strings.Join(fps, "\n") + "\n\n"
}

// opensslForm renders a fingerprint the way `openssl x509 -noout -fingerprint -sha256` prints it.
func opensslForm(fp string) string {
	upper := strings.ToUpper(fp)
	parts := make([]string, 0, len(upper)/2)
	for i := 0; i < len(upper); i += 2 {
		parts = append(parts, upper[i:i+2])
	}
	return "sha256 Fingerprint=" + strings.Join(parts, ":")
}

// testOutput returns an Output whose informational and warning/error lines are captured. Call the returned function to read
// them. It swaps os.Stderr while constructing, so tests in this file must not run in parallel.
func testOutput(t *testing.T) (*output.Output, func() (info, stderr string)) {
	t.Helper()
	dir := t.TempDir()
	infoPath := filepath.Join(dir, "info")
	errPath := filepath.Join(dir, "stderr")
	errFile, err := os.Create(errPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { errFile.Close() })

	prevStderr := os.Stderr
	os.Stderr = errFile
	out := output.New(infoPath, false)
	os.Stderr = prevStderr

	return out, func() (string, string) {
		info, err := os.ReadFile(infoPath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		stderr, err := os.ReadFile(errPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(info), string(stderr)
	}
}

func tmpFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".hnvm-tmp") {
			names = append(names, e.Name())
		}
	}
	return names
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertUnpatched(t *testing.T, fx *fixture) {
	t.Helper()
	if !bytes.Equal(readFile(t, fx.nodeBin), fx.orig) {
		t.Error("node binary was modified")
	}
	if _, err := os.Stat(fx.markerFile); !os.IsNotExist(err) {
		t.Error("marker file was written")
	}
	if got := tmpFiles(t, fx.binDir); len(got) != 0 {
		t.Errorf("temporary files left behind: %v", got)
	}
	if _, err := os.Stat(fx.lockDir); !os.IsNotExist(err) {
		t.Error("lock directory left behind")
	}
}

func TestApplyRemovesDistrustedRootsAndRecordsMarker(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	b := newTestCert(t, strings.Repeat("b", 200))
	c := newTestCert(t, strings.Repeat("c", 200))
	fx := newFixture(t, []testCert{a, b, c}, a.fp, c.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	list := listOf(a.fp)
	extra := opensslForm(c.fp)
	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true, Extra: extra}, list, fx.placeholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	info, stderr := read()
	if !strings.Contains(info, "Removed 2 distrusted root certificate(s) from node v18.12.0") {
		t.Errorf("info = %q, want removal message", info)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}

	patched := readFile(t, fx.nodeBin)
	if len(patched) != len(fx.orig) {
		t.Errorf("binary size changed: %d -> %d", len(fx.orig), len(patched))
	}
	placeholderFP := pemFingerprint(bytes.TrimSpace([]byte(fx.placeholder)))
	got := blockFingerprints(patched)
	want := []string{placeholderFP, b.fp, placeholderFP}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("block fingerprints = %v, want %v", got, want)
	}
	if fi, err := os.Stat(fx.nodeBin); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v (%v), want 0755", fi.Mode().Perm(), err)
	}
	if got := tmpFiles(t, fx.binDir); len(got) != 0 {
		t.Errorf("temporary files left behind: %v", got)
	}
	if _, err := os.Stat(fx.lockDir); !os.IsNotExist(err) {
		t.Error("lock directory left behind")
	}

	wantMarker := "v1\n# test list\n" + a.fp + "\nextra=" + extra + "\n"
	if got := string(readFile(t, fx.markerFile)); got != wantMarker {
		t.Errorf("marker = %q, want %q", got, wantMarker)
	}
	if fx.fake.verifyCalls != 1 || fx.fake.rootCalls != 1 {
		t.Errorf("verify calls = %d, root calls = %d, want 1 and 1", fx.fake.verifyCalls, fx.fake.rootCalls)
	}
}

func TestApplyIsIdempotentViaMarker(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	b := newTestCert(t, strings.Repeat("b", 200))
	fx := newFixture(t, []testCert{a, b}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatal(err)
	}
	patched := readFile(t, fx.nodeBin)
	verifies, roots := fx.fake.verifyCalls, fx.fake.rootCalls

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatal(err)
	}
	info, stderr := read()
	if strings.Count(info, "Removed") != 1 || stderr != "" {
		t.Errorf("second run output: info=%q stderr=%q", info, stderr)
	}
	if !bytes.Equal(readFile(t, fx.nodeBin), patched) {
		t.Error("binary changed on second run")
	}
	if fx.fake.verifyCalls != verifies || fx.fake.rootCalls != roots {
		t.Error("second run patched again")
	}
}

func TestApplyInvalidFingerprintsWarnsAndStillRemovesValid(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	b := newTestCert(t, strings.Repeat("b", 200))
	fx := newFixture(t, []testCert{a, b}, a.fp, b.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	extra := "not-a-fingerprint," + b.fp
	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true, Extra: extra}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, stderr := read()
	wantWarning := "node v18.12.0 still trusts distrusted root certificates: invalid SHA-256 fingerprint(s) in HNVM_DISTRUST_CERTS_EXTRA: not-a-fingerprint"
	if !strings.Contains(stderr, wantWarning) {
		t.Errorf("stderr = %q, want %q", stderr, wantWarning)
	}
	if !strings.Contains(info, "Removed 2 distrusted root certificate(s)") {
		t.Errorf("info = %q, want both valid roots removed", info)
	}
	wantMarker := "v1\n# test list\n" + a.fp + "\nextra=" + extra + "\ninvalid=not-a-fingerprint\n"
	if got := string(readFile(t, fx.markerFile)); got != wantMarker {
		t.Errorf("marker = %q, want %q", got, wantMarker)
	}

	// Later runs repeat the warning without patching again
	verifies := fx.fake.verifyCalls
	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true, Extra: extra}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatal(err)
	}
	info, stderr = read()
	if strings.Count(stderr, "invalid SHA-256 fingerprint(s)") != 2 {
		t.Errorf("warning not repeated: stderr = %q", stderr)
	}
	if strings.Count(info, "Removed") != 1 {
		t.Errorf("second run re-patched: info = %q", info)
	}
	if fx.fake.verifyCalls != verifies {
		t.Error("second run verified (patched) again")
	}
}

func TestApplyInvalidFingerprintsStrictRefuses(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true, Strict: true, Extra: "not-a-fingerprint"}, listOf(a.fp), fx.placeholder, out)
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	_, stderr := read()
	want := "Unable to remove distrusted root certificates from node v18.12.0: invalid SHA-256 fingerprint(s) in HNVM_DISTRUST_CERTS_EXTRA: not-a-fingerprint"
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, "Set HNVM_DISTRUST_CERTS=false to run this node anyway") {
		t.Errorf("stderr = %q", stderr)
	}
	assertUnpatched(t, fx)
	if fx.fake.verifyCalls != 0 {
		t.Error("strict refusal still patched")
	}
}

func TestApplyManagedWordingMentionsOrganization(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	s := Settings{Enabled: true, Strict: true, Extra: "not-a-fingerprint", ExtraManaged: true, StrictManaged: true}
	if err := Apply(fx.nodeDir, testVersion, s, listOf(a.fp), fx.placeholder, out); !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "HNVM_DISTRUST_CERTS_EXTRA (yours or your organization's): not-a-fingerprint") {
		t.Errorf("stderr = %q, want managed extra wording", stderr)
	}
	if !strings.Contains(stderr, "Your organization requires this, so node will not run until they can be removed.") {
		t.Errorf("stderr = %q, want managed refusal wording", stderr)
	}
}

func TestApplyPlaceholderTooLarge(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, strings.Repeat("p", 800)).block)
	out, read := testOutput(t)

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatalf("non-strict Apply: %v", err)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "node v18.12.0 still trusts distrusted root certificates: Placeholder certificate (") ||
		!strings.Contains(stderr, "is larger than a") {
		t.Errorf("stderr = %q", stderr)
	}
	assertUnpatched(t, fx)

	s := Settings{Enabled: true, Strict: true}
	if err := Apply(fx.nodeDir, testVersion, s, listOf(a.fp), fx.placeholder, out); !errors.Is(err, ErrRefused) {
		t.Fatalf("strict err = %v, want ErrRefused", err)
	}
}

func TestApplyNoBlocksInBinary(t *testing.T) {
	fx := newFixture(t, nil)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf("00"), fx.placeholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	_, stderr := read()
	want := "Could not find any bundled root certificates in " + fx.nodeBin
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	assertUnpatched(t, fx)
}

func TestApplyUnexpectedRootCountOutput(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	for _, tc := range []struct{ output, want string }{
		{"", "unexpected output from " + fx.nodeBin + ": (none)"},
		{"garbage\n", "unexpected output from " + fx.nodeBin + ": garbage"},
	} {
		fx.fake.rootsOutput = &tc.output
		if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		_, stderr := read()
		want := "node v18.12.0 still trusts distrusted root certificates: " + tc.want
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
		assertUnpatched(t, fx)
	}
}

func TestApplyVerifyFailureKeepsOriginal(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	fx.fake.verifyErr = errors.New("exit status 1")
	out, read := testOutput(t)

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "the patched binary failed verification: verify exploded") {
		t.Errorf("stderr = %q", stderr)
	}
	assertUnpatched(t, fx)
}

func TestApplyUnwritableBinDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	if err := os.Chmod(fx.binDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(fx.binDir, 0o755) })
	out, read := testOutput(t)

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatalf("non-strict Apply: %v", err)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "node v18.12.0 still trusts distrusted root certificates: could not create a temporary file next to") {
		t.Errorf("stderr = %q", stderr)
	}
	assertUnpatched(t, fx)

	err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true, Strict: true}, listOf(a.fp), fx.placeholder, out)
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("strict err = %v, want ErrRefused", err)
	}
	if fx.fake.verifyCalls != 0 {
		t.Error("ran verify without a temporary file")
	}
}

func TestApplyLockContentionPatchesOnce(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	b := newTestCert(t, strings.Repeat("b", 200))
	fx := newFixture(t, []testCert{a, b}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	const workers = 8
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("worker %d: %v", i, err)
		}
	}
	info, stderr := read()
	if strings.Count(info, "Removed 1 distrusted") != 1 {
		t.Errorf("info = %q, want exactly one patch", info)
	}
	if stderr != "" {
		t.Errorf("stderr = %q", stderr)
	}
	if fx.fake.verifyCalls != 1 || fx.fake.rootCalls != 1 {
		t.Errorf("verify = %d, root = %d, want 1 and 1", fx.fake.verifyCalls, fx.fake.rootCalls)
	}
	if got := tmpFiles(t, fx.binDir); len(got) != 0 {
		t.Errorf("temporary files left behind: %v", got)
	}
	if _, err := os.Stat(fx.lockDir); !os.IsNotExist(err) {
		t.Error("lock directory left behind")
	}
	if fps := blockFingerprints(readFile(t, fx.nodeBin)); contains(fps, a.fp) {
		t.Error("distrusted root still present")
	}
}

func TestApplyRemovesStaleLock(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	if err := os.Mkdir(fx.lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(fx.lockDir, old, old); err != nil {
		t.Fatal(err)
	}
	out, read := testOutput(t)

	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, stderr := read()
	if !strings.Contains(info, "Removed 1 distrusted") || stderr != "" {
		t.Errorf("info = %q, stderr = %q", info, stderr)
	}
	if _, err := os.Stat(fx.lockDir); !os.IsNotExist(err) {
		t.Error("lock directory left behind")
	}
}

func TestApplyWaitsForFreshLock(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	fx := newFixture(t, []testCert{a}, a.fp)
	fx.placeholder = string(newTestCert(t, "p").block)
	if err := os.Mkdir(fx.lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, read := testOutput(t)

	done := make(chan error, 1)
	go func() {
		done <- Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(a.fp), fx.placeholder, out)
	}()

	select {
	case err := <-done:
		t.Fatalf("Apply returned while a fresh lock was held: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if v, _ := fx.fake.calls(); v != 0 {
		t.Fatal("patched while another process held the lock")
	}
	if err := os.Remove(fx.lockDir); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Apply did not finish after the lock was released")
	}
	if info, _ := read(); !strings.Contains(info, "Removed 1 distrusted") {
		t.Errorf("info = %q", info)
	}
}

func TestApplyNothingToRemoveWritesMarker(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	b := newTestCert(t, strings.Repeat("b", 200))
	fx := newFixture(t, []testCert{a, b})
	fx.placeholder = string(newTestCert(t, "p").block)
	out, read := testOutput(t)

	// A valid fingerprint that is not in the binary, so nothing matches.
	absent := newTestCert(t, "absent")
	if err := Apply(fx.nodeDir, testVersion, Settings{Enabled: true}, listOf(absent.fp), fx.placeholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, stderr := read()
	if info != "" || stderr != "" {
		t.Errorf("info = %q, stderr = %q, want both empty", info, stderr)
	}
	if !bytes.Equal(readFile(t, fx.nodeBin), fx.orig) {
		t.Error("binary modified")
	}
	if got := tmpFiles(t, fx.binDir); len(got) != 0 {
		t.Errorf("temporary files left behind: %v", got)
	}
	if _, err := os.Stat(fx.markerFile); err != nil {
		t.Errorf("marker not written: %v", err)
	}
}

func TestNormalizeFingerprints(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	for _, tc := range []struct {
		name, list, extra string
		want              []string
	}{
		{"comments and blanks skipped", "# c\n\n   \n" + fp + "\n", "", []string{fp}},
		{"openssl form", "", opensslForm(fp), []string{fp}},
		{"case and colons", "", strings.ToUpper(strings.ReplaceAll(fp, "ab", "aB:")), []string{fp}},
		{"dedupe and sort", "", "ff," + fp + "," + fp, []string{fp, "ff"}},
		{"empty entries dropped", "", " ,,", nil},
		{"invalid kept", "", "not-a-fingerprint", []string{"not-a-fingerprint"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeFingerprints(tc.list, tc.extra)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPemFingerprintMatchesDER(t *testing.T) {
	c := newTestCert(t, strings.Repeat("x", 150))
	if got := pemFingerprint(c.block); got != c.fp {
		t.Errorf("pemFingerprint = %s, want %s", got, c.fp)
	}
	// Whitespace inside the body must not change the result. The scan only ever passes exact BEGIN..END slices, so the
	// trailing newline that EncodeToMemory adds is trimmed first.
	spaced := bytes.ReplaceAll(bytes.TrimSpace(c.block), []byte("\n"), []byte("\r\n  "))
	if got := pemFingerprint(spaced); got != c.fp {
		t.Errorf("pemFingerprint with whitespace = %s, want %s", got, c.fp)
	}
}

func TestPatchBinaryPadsWithNewlines(t *testing.T) {
	a := newTestCert(t, strings.Repeat("a", 200))
	placeholder := []byte("PLACEHOLDER-" + strings.Repeat("x", 4))
	bin := append(append([]byte("junk"), a.block...), []byte("junk")...)
	length := len(a.block)

	removed, blocks, err := patchBinary(bin, "test", map[string]bool{a.fp: true}, placeholder)
	if err != nil || removed != 1 || blocks != 1 {
		t.Fatalf("patchBinary = %d, %d, %v", removed, blocks, err)
	}
	region := bin[4 : 4+length]
	if !bytes.HasPrefix(region, placeholder) {
		t.Errorf("region does not start with placeholder: %q", region[:len(placeholder)])
	}
	if !bytes.Equal(region[len(placeholder):], bytes.Repeat([]byte("\n"), length-len(placeholder))) {
		t.Error("padding is not all newlines")
	}
	if !bytes.Equal(bin[:4], []byte("junk")) || !bytes.HasSuffix(bin, []byte("junk")) {
		t.Error("bytes outside the block changed")
	}
}

func TestDefaultRunnerReturnsCombinedOutput(t *testing.T) {
	out, err := execCommand("sh", "-c", "echo out; echo err 1>&2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "out") || !strings.Contains(string(out), "err") {
		t.Errorf("output = %q", out)
	}
}

// TestApplyWithRealNode copies the 18.12.0 install from ~/.hnvm into a temp dir and patches the copy with the real runner.
// Skipped when that install is not present. The original is only read.
func TestApplyWithRealNode(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	src := filepath.Join(home, ".hnvm", "node", testVersion)
	if _, err := os.Stat(filepath.Join(src, "bin", "node")); err != nil {
		t.Skipf("no node %s install at %s", testVersion, src)
	}

	dir := t.TempDir()
	nodeDir := filepath.Join(dir, "node", testVersion)
	if err := os.MkdirAll(filepath.Dir(nodeDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-a", src, nodeDir).CombinedOutput(); err != nil {
		t.Fatalf("copy: %v: %s", err, out)
	}
	// A marker from a real hnvm run would make Apply a no-op, so start from a clean copy.
	_ = os.Remove(filepath.Join(nodeDir, markerFileName))
	_ = os.Remove(filepath.Join(nodeDir, lockDirName))

	// The real runner is used here; restore it in case another test replaced it.
	prev := execCommand
	execCommand = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
	t.Cleanup(func() { execCommand = prev })

	out, read := testOutput(t)
	if err := Apply(nodeDir, testVersion, Settings{Enabled: true}, hnvm.DistrustedCerts, hnvm.DistrustedPlaceholder, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, stderr := read()
	if !strings.Contains(info, "Removed 2 distrusted root certificate(s) from node v18.12.0") {
		t.Errorf("info = %q, stderr = %q", info, stderr)
	}
	if _, err := os.Stat(filepath.Join(nodeDir, markerFileName)); err != nil {
		t.Errorf("marker not written: %v", err)
	}

	// A second run finds the marker and does nothing
	before, _ := read()
	if err := Apply(nodeDir, testVersion, Settings{Enabled: true}, hnvm.DistrustedCerts, hnvm.DistrustedPlaceholder, out); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if after, _ := read(); after != before {
		t.Errorf("second run printed %q", strings.TrimPrefix(after, before))
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
