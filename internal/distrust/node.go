package distrust

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// execCommand runs a program and returns its combined stdout and stderr. It is a variable so tests can fake node.
// The environment is inherited unchanged (NODE_OPTIONS included), like the bash version.
var execCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// rootCountScript prints the number of root certificates the running node bundles, or -1 when this node can't report them
// (tls.rootCertificates was added in node 12.16.0 and 13.9.0).
const rootCountScript = `
var tls = require('tls');
process.stdout.write('hnvm-distrust-roots ' + (tls.rootCertificates ? tls.rootCertificates.length : -1) + '\n');
`

var rootCountLine = regexp.MustCompile(`^hnvm-distrust-roots (-?[0-9]+)$`)

// rootCount asks the original node binary how many roots it bundles. Any output without a well-formed result line is an error,
// so a broken or silenced node never gets its roots recorded as removed.
func (a *applier) rootCount() (int, error) {
	out, err := execCommand(a.nodeBin, "-e", rootCountScript)
	text := trimOutput(out)
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return 0, fmt.Errorf("%s", text)
	}
	count, found := 0, false
	for _, line := range strings.Split(text, "\n") {
		if m := rootCountLine.FindStringSubmatch(line); m != nil {
			count, _ = strconv.Atoi(m[1])
			found = true
		}
	}
	if !found {
		if text == "" {
			text = "(none)"
		}
		return 0, fmt.Errorf("unexpected output from %s: %s", a.nodeBin, text)
	}
	return count, nil
}

// verifyTemplate is run by the patched binary (with -e). It checks the binary has the expected number of PEM blocks, none of
// them distrusted, that it builds a TLS context, and (where node can report them) that its bundled roots have the expected
// count and none of them are distrusted. It exits non-zero on any mismatch. The placeholders are filled by verifyScript.
const verifyTemplate = `
var fs = require('fs');
var crypto = require('crypto');
var tls = require('tls');
var distrusted = new Set([__FPS__]);
var BEGIN = '-----BEGIN CERTIFICATE-----';
var END = '-----END CERTIFICATE-----';
function fail(message) {
  process.stderr.write(message + '\n');
  process.exit(1);
}
function fingerprint(pem) {
  var body = pem.slice(BEGIN.length, pem.length - END.length).replace(/\s+/g, '');
  return crypto.createHash('sha256').update(Buffer.from(body, 'base64')).digest('hex');
}
var binary = fs.readFileSync(process.execPath);
var begin = Buffer.from(BEGIN, 'latin1');
var end = Buffer.from(END, 'latin1');
var blocks = 0;
var distrustedFound = 0;
var start = binary.indexOf(begin);
while (start !== -1) {
  var endIndex = binary.indexOf(end, start);
  if (endIndex === -1) break;
  var stop = endIndex + end.length;
  blocks++;
  if (distrusted.has(fingerprint(binary.toString('latin1', start, stop)))) distrustedFound++;
  start = binary.indexOf(begin, stop);
}
if (blocks !== __BLOCKS__) {
  fail('Expected __BLOCKS__ PEM blocks in ' + process.execPath + ', found ' + blocks);
}
if (distrustedFound > 0) {
  fail(distrustedFound + ' distrusted root certificate(s) still in ' + process.execPath);
}
tls.createSecureContext();
if (tls.rootCertificates) {
  var roots = tls.rootCertificates.length;
  if (roots !== __ROOTS__) {
    fail('Expected __ROOTS__ bundled roots, found ' + roots);
  }
  if (tls.rootCertificates.some(function (pem) { return distrusted.has(fingerprint(pem.trim())); })) {
    fail('A distrusted root certificate is still bundled');
  }
}
`

// verifyScript builds the verify program for the given fingerprints (lowercase hex), expected PEM block count and root count.
func verifyScript(fps []string, blocks, roots int) string {
	quoted := make([]string, len(fps))
	for i, fp := range fps {
		quoted[i] = "'" + fp + "'"
	}
	return strings.NewReplacer(
		"__FPS__", strings.Join(quoted, ", "),
		"__BLOCKS__", fmt.Sprint(blocks),
		"__ROOTS__", fmt.Sprint(roots),
	).Replace(verifyTemplate)
}

// trimOutput returns command output without trailing newlines, as $(...) would.
func trimOutput(out []byte) string {
	return strings.TrimRight(string(out), "\n")
}
