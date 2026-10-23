// Removes distrusted root CA certificates from a Node.js binary's bundled root store.
//
// Node compiles its root CAs into the binary as PEM string literals. Each distrusted PEM block is
// overwritten in place with a harmless, already-expired placeholder certificate, padded with
// newlines to the exact original length. The binary layout is untouched and Node still parses every
// root entry, it just no longer trusts the distrusted ones.
//
// Must be run by the node binary being patched, so it can report that binary's root count. Sticks to
// APIs available in every node version hnvm can install: tls.rootCertificates only exists from Node
// 12.16.0/13.9.0, so the binary itself is scanned as well and that check is skipped when it's missing.
//
// Usage:
//   node distrust-certs.js patch <src binary> <dest file> <placeholder.pem> <fp1,fp2,...>
//     Writes the patched binary to <dest file> (only when something matched) and prints
//     "<number of certs removed> <number of PEM blocks in the binary> <number of bundled roots>",
//     where the root count is -1 if this node can't report it.
//   node distrust-certs.js verify <fp1,fp2,...> <expected PEM blocks> <expected bundled roots>
//     Exits non-zero unless this node's binary has the expected number of PEM blocks, none of them
//     match a distrusted fingerprint, it builds a TLS context and (where this node can report them)
//     its bundled roots have the expected count and none of them match either.
const fs = require('fs')
const crypto = require('crypto')
const tls = require('tls')

const BEGIN = '-----BEGIN CERTIFICATE-----'
const END = '-----END CERTIFICATE-----'

function fail(message) {
  process.stderr.write(`${message}\n`)
  process.exit(1)
}

// Same normalization as ensure_bin.sh: drops a leading "sha256 Fingerprint=" label (as printed by
// `openssl x509 -fingerprint -sha256`), colons, whitespace and case
function parseFingerprints(list) {
  const fingerprints = list
    .split(',')
    .map(fp => fp.replace(/^.*=/, '').replace(/[:\s]/g, '').toLowerCase())
    .filter(Boolean)
  const invalid = fingerprints.filter(fp => !/^[0-9a-f]{64}$/.test(fp))
  if (invalid.length > 0) {
    fail(`Invalid SHA-256 fingerprint(s): ${invalid.join(', ')}`)
  }
  return new Set(fingerprints)
}

function pemFingerprint(pem) {
  const body = pem.slice(BEGIN.length, pem.length - END.length).replace(/\s+/g, '')
  return crypto.createHash('sha256').update(Buffer.from(body, 'base64')).digest('hex')
}

// Calls onBlock(fingerprint, start, stop) for every PEM certificate in binary, returns how many there are
function forEachPem(binary, onBlock) {
  const begin = Buffer.from(BEGIN, 'latin1')
  const end = Buffer.from(END, 'latin1')

  let blocks = 0
  let start = binary.indexOf(begin)
  while (start !== -1) {
    const endIndex = binary.indexOf(end, start)
    if (endIndex === -1) break
    const stop = endIndex + end.length
    blocks++
    onBlock(pemFingerprint(binary.toString('latin1', start, stop)), start, stop)
    start = binary.indexOf(begin, stop)
  }
  return blocks
}

function rootCount() {
  return tls.rootCertificates ? tls.rootCertificates.length : -1
}

function patch(src, dest, placeholderFile, fingerprintList) {
  const fingerprints = parseFingerprints(fingerprintList)
  const placeholder = fs.readFileSync(placeholderFile, 'latin1').trim()
  const binary = fs.readFileSync(src)

  let removed = 0
  const blocks = forEachPem(binary, (fingerprint, start, stop) => {
    if (fingerprints.has(fingerprint)) {
      const length = stop - start
      if (placeholder.length > length) {
        fail(`Placeholder certificate (${placeholder.length} bytes) is larger than a ${length} byte root`)
      }
      binary.write(placeholder.padEnd(length, '\n'), start, length, 'latin1')
      removed++
    }
  })

  if (blocks === 0) {
    fail(`Could not find any bundled root certificates in ${src}`)
  }

  if (removed > 0) {
    // dest is a fresh file from mktemp, created without execute permissions
    fs.writeFileSync(dest, binary)
    fs.chmodSync(dest, fs.statSync(src).mode)
  }

  process.stdout.write(`${removed} ${blocks} ${rootCount()}\n`)
}

function verify(fingerprintList, expectedBlocks, expectedRoots) {
  const fingerprints = parseFingerprints(fingerprintList)

  let distrusted = 0
  const blocks = forEachPem(fs.readFileSync(process.execPath), fingerprint => {
    if (fingerprints.has(fingerprint)) distrusted++
  })
  if (blocks !== Number(expectedBlocks)) {
    fail(`Expected ${expectedBlocks} PEM blocks in ${process.execPath}, found ${blocks}`)
  }
  if (distrusted > 0) {
    fail(`${distrusted} distrusted root certificate(s) still in ${process.execPath}`)
  }

  // Parses every bundled root, node aborts if one of them is malformed
  tls.createSecureContext()

  if (tls.rootCertificates) {
    if (rootCount() !== Number(expectedRoots)) {
      fail(`Expected ${expectedRoots} bundled roots, found ${rootCount()}`)
    }
    if (tls.rootCertificates.some(pem => fingerprints.has(pemFingerprint(pem.trim())))) {
      fail('A distrusted root certificate is still bundled')
    }
  }
}

const [mode, ...args] = process.argv.slice(2)
if (mode === 'patch' && args.length === 4) {
  patch(...args)
} else if (mode === 'verify' && args.length === 3) {
  verify(...args)
} else {
  fail('Usage: distrust-certs.js patch <src> <dest> <placeholder.pem> <fingerprints> | verify <fingerprints> <expected blocks> <expected roots>')
}
