// Removes distrusted root CA certificates from a Node.js binary's bundled root store.
//
// Node compiles its root CAs into the binary as PEM string literals. Each distrusted PEM block is
// overwritten in place with a harmless, already-expired placeholder certificate, padded with
// newlines to the exact original length. The binary layout is untouched and Node still parses every
// root entry, it just no longer trusts the distrusted ones.
//
// Must be run by the node binary being patched, so it can report that binary's root count.
//
// Usage:
//   node distrust-certs.js patch <src binary> <dest file> <placeholder.pem> <fp1,fp2,...>
//     Writes the patched binary to <dest file> (only when something matched) and prints
//     "<number of certs removed> <number of bundled roots>".
//   node distrust-certs.js verify <fp1,fp2,...> <expected number of bundled roots>
//     Exits non-zero unless this node builds a TLS context, has the expected number of bundled
//     roots and none of them match a distrusted fingerprint.
const fs = require('fs')
const crypto = require('crypto')
const tls = require('tls')

const BEGIN = '-----BEGIN CERTIFICATE-----'
const END = '-----END CERTIFICATE-----'

function fail(message) {
  process.stderr.write(`${message}\n`)
  process.exit(1)
}

function parseFingerprints(list) {
  return new Set(
    list
      .split(',')
      .map(fp => fp.replace(/:/g, '').trim().toLowerCase())
      .filter(Boolean),
  )
}

function pemFingerprint(pem) {
  const body = pem.slice(BEGIN.length, pem.length - END.length).replace(/\s+/g, '')
  return crypto.createHash('sha256').update(Buffer.from(body, 'base64')).digest('hex')
}

function patch(src, dest, placeholderFile, fingerprintList) {
  const fingerprints = parseFingerprints(fingerprintList)
  const placeholder = fs.readFileSync(placeholderFile, 'latin1').trim()
  const binary = fs.readFileSync(src)
  const begin = Buffer.from(BEGIN, 'latin1')
  const end = Buffer.from(END, 'latin1')

  let blocks = 0
  let removed = 0
  let start = binary.indexOf(begin)
  while (start !== -1) {
    const endIndex = binary.indexOf(end, start)
    if (endIndex === -1) break
    const stop = endIndex + end.length
    blocks++

    if (fingerprints.has(pemFingerprint(binary.toString('latin1', start, stop)))) {
      const length = stop - start
      if (placeholder.length > length) {
        fail(`Placeholder certificate (${placeholder.length} bytes) is larger than a ${length} byte root`)
      }
      binary.write(placeholder.padEnd(length, '\n'), start, length, 'latin1')
      removed++
    }

    start = binary.indexOf(begin, stop)
  }

  if (blocks === 0) {
    fail(`Could not find any bundled root certificates in ${src}`)
  }

  if (removed > 0) {
    // Clear any leftover from an interrupted run, writeFileSync only applies `mode` to new files
    try {
      fs.unlinkSync(dest)
    } catch (_) {}
    fs.writeFileSync(dest, binary, {mode: fs.statSync(src).mode})
  }

  process.stdout.write(`${removed} ${tls.rootCertificates.length}\n`)
}

function verify(fingerprintList, expectedRoots) {
  const fingerprints = parseFingerprints(fingerprintList)
  tls.createSecureContext()

  if (tls.rootCertificates.length !== Number(expectedRoots)) {
    fail(`Expected ${expectedRoots} bundled roots, found ${tls.rootCertificates.length}`)
  }
  if (tls.rootCertificates.some(pem => fingerprints.has(pemFingerprint(pem.trim())))) {
    fail('A distrusted root certificate is still bundled')
  }
}

const [mode, ...args] = process.argv.slice(2)
if (mode === 'patch' && args.length === 4) {
  patch(...args)
} else if (mode === 'verify' && args.length === 2) {
  verify(...args)
} else {
  fail('Usage: distrust-certs.js patch <src> <dest> <placeholder.pem> <fingerprints> | verify <fingerprints> <expected roots>')
}
