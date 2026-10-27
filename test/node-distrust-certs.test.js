const fs = require('fs')
const path = require('path')
const childProcess = require('child_process')
const {createTestContext, describeUnlessManaged} = require('./utils.js')

jest.setTimeout(120_000)

const EMSIGN_ECC_G3 = '86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b'
const AAA_CERTIFICATE_SERVICES = 'd7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4'

// Prints the SHA-256 fingerprints of every root certificate bundled into the running node
// (tls.rootCertificates needs node 12.16.0+)
const PRINT_ROOT_FINGERPRINTS = `
  require('tls').rootCertificates.map(pem => require('crypto').createHash('sha256')
    .update(Buffer.from(pem.replace(/-----[A-Z ]+-----|\\s/g, ''), 'base64')).digest('hex')).join(',')
`

// Same, but read straight out of the running node binary, for any node version
const PRINT_BINARY_FINGERPRINTS = `
  (require('fs').readFileSync(process.execPath, 'latin1')
    .match(/-----BEGIN CERTIFICATE-----[\\s\\S]*?-----END CERTIFICATE-----/g) || [])
    .map(pem => require('crypto').createHash('sha256')
      .update(Buffer.from(pem.replace(/-----[A-Z ]+-----|\\s/g, ''), 'base64')).digest('hex')).join(',')
`

// As printed by `openssl x509 -noout -fingerprint -sha256`
function opensslFingerprint(fingerprint) {
  return `sha256 Fingerprint=${fingerprint.toUpperCase().match(/../g).join(':')}`
}

function tmpFiles(dir) {
  return fs.readdirSync(dir).filter(file => file.includes('.hnvm-tmp'))
}

describeUnlessManaged('removing distrusted root certificates from node', () => {
  // 18.12.0 bundles both of the eMudhra-affected roots
  const VERSION = '18.12.0'
  let context
  let nodeBinDir
  let markerFile

  function rootFingerprints(extraEnv) {
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['-p', PRINT_ROOT_FINGERPRINTS], extraEnv)
    expect(result.status).toBe(0)
    return {fingerprints: result.stdout.trim().split(','), stderr: result.stderr}
  }

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: VERSION}})
    nodeBinDir = path.join(context.hnvmDir, 'node', VERSION, 'bin')
    markerFile = path.join(context.hnvmDir, 'node', VERSION, '.hnvm-distrusted-certs')
  })

  afterAll(() => {
    fs.chmodSync(nodeBinDir, 0o755)
    context.cleanup()
  })

  it('leaves the bundled roots alone when HNVM_DISTRUST_CERTS=false', () => {
    const {fingerprints, stderr} = rootFingerprints({HNVM_DISTRUST_CERTS: 'false'})

    expect(fingerprints).toContain(EMSIGN_ECC_G3)
    expect(fingerprints).toContain(AAA_CERTIFICATE_SERVICES)
    expect(stderr).not.toContain('distrusted')
    expect(fs.existsSync(markerFile)).toBe(false)
  })

  describe('when the node binary cannot be patched', () => {
    beforeAll(() => {
      fs.chmodSync(nodeBinDir, 0o555)
    })

    afterAll(() => {
      fs.chmodSync(nodeBinDir, 0o755)
    })

    it('warns and still runs node by default', () => {
      const {fingerprints, stderr} = rootFingerprints()

      expect(stderr).toContain(`node v${VERSION} still trusts distrusted root certificates`)
      expect(fingerprints).toContain(EMSIGN_ECC_G3)
      expect(fs.existsSync(markerFile)).toBe(false)
      expect(tmpFiles(nodeBinDir)).toEqual([])
    })

    it('refuses to run node when HNVM_DISTRUST_CERTS_STRICT=true', () => {
      const result = context.execFileSyncSeparateStreams(context.binaries.node, ['--version'], {
        HNVM_DISTRUST_CERTS_STRICT: 'true',
      })

      expect(result.status).not.toBe(0)
      expect(result.stdout).not.toContain(VERSION)
      expect(result.stderr).toContain(`Unable to remove distrusted root certificates from node v${VERSION}`)
    })
  })

  it('does not record the roots as removed when the patch result is missing', () => {
    // Swallows everything node writes to stdout, including distrust-certs.js's result line
    const silenceStdout = path.join(context.testDir, 'silence-stdout.js')
    fs.writeFileSync(silenceStdout, 'process.stdout.write = () => true\n')

    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['-e', '0'], {
      NODE_OPTIONS: `--require ${silenceStdout}`,
    })

    expect(result.status).toBe(0)
    expect(result.stderr).toContain(`node v${VERSION} still trusts distrusted root certificates: unexpected output`)
    expect(fs.existsSync(markerFile)).toBe(false)
    expect(rootFingerprints({HNVM_DISTRUST_CERTS: 'false'}).fingerprints).toContain(EMSIGN_ECC_G3)
  })

  it('removes the distrusted roots from an already installed node by default', () => {
    const before = rootFingerprints({HNVM_DISTRUST_CERTS: 'false'}).fingerprints
    const {fingerprints, stderr} = rootFingerprints()

    expect(stderr).toContain(`Removed 2 distrusted root certificate(s) from node v${VERSION}`)
    expect(fingerprints).not.toContain(EMSIGN_ECC_G3)
    expect(fingerprints).not.toContain(AAA_CERTIFICATE_SERVICES)
    // Every other root is still trusted
    expect(fingerprints.length).toBe(before.length)
    expect(before.filter(fp => !fingerprints.includes(fp)).sort()).toEqual(
      [EMSIGN_ECC_G3, AAA_CERTIFICATE_SERVICES].sort(),
    )
    expect(fs.existsSync(markerFile)).toBe(true)
  })

  it('does not patch the same node again on later runs', () => {
    const output = context.execFileSync(context.binaries.node, ['--version'])

    expect(output).toContain(VERSION)
    expect(output).not.toContain('Removed')
  })

  it('also removes roots listed in HNVM_DISTRUST_CERTS_EXTRA', () => {
    const markerBefore = fs.readFileSync(markerFile, 'utf-8')
    const extra = rootFingerprints().fingerprints.find(
      fp => fp !== EMSIGN_ECC_G3 && fp !== AAA_CERTIFICATE_SERVICES,
    )
    const {fingerprints, stderr} = rootFingerprints({HNVM_DISTRUST_CERTS_EXTRA: opensslFingerprint(extra)})

    expect(stderr).toContain(`Removed 1 distrusted root certificate(s) from node v${VERSION}`)
    expect(fingerprints).not.toContain(extra)
    expect(fs.readFileSync(markerFile, 'utf-8')).not.toEqual(markerBefore)
  })
  it('rejects invalid fingerprints in HNVM_DISTRUST_CERTS_EXTRA without recording them', () => {
    const markerBefore = fs.readFileSync(markerFile, 'utf-8')

    const warned = rootFingerprints({HNVM_DISTRUST_CERTS_EXTRA: 'not-a-fingerprint'})
    expect(warned.stderr).toContain('invalid SHA-256 fingerprint(s) in HNVM_DISTRUST_CERTS_EXTRA: not-a-fingerprint')

    const strict = context.execFileSyncSeparateStreams(context.binaries.node, ['--version'], {
      HNVM_DISTRUST_CERTS_EXTRA: 'not-a-fingerprint',
      HNVM_DISTRUST_CERTS_STRICT: 'true',
    })
    expect(strict.status).not.toBe(0)
    expect(strict.stdout).not.toContain(VERSION)

    expect(fs.readFileSync(markerFile, 'utf-8')).toEqual(markerBefore)
  })
})

describeUnlessManaged('removing distrusted root certificates from node without tls.rootCertificates', () => {
  // tls.rootCertificates was added in node 12.16.0. 10.24.1 bundles AAA Certificate Services, plus one
  // more of its roots is removed through HNVM_DISTRUST_CERTS_EXTRA.
  const VERSION = '10.24.1'
  let context
  let nodeBinDir
  let nodePath
  let extra

  function binaryFingerprints(extraEnv) {
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['-p', PRINT_BINARY_FINGERPRINTS], extraEnv)
    expect(result.status).toBe(0)
    return {fingerprints: result.stdout.trim().split(','), stderr: result.stderr}
  }

  function runNode(extraEnv) {
    return new Promise(resolve => {
      const child = childProcess.execFile(
        context.binaries.node,
        ['-p', 'process.version'],
        {env: {HNVM_PATH: context.hnvmDir, PATH: process.env.PATH, ...extraEnv}, cwd: context.cwdDir},
        (error, stdout, stderr) => resolve({status: error ? error.code : 0, stdout, stderr}),
      )
      child.stdin.end()
    })
  }

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: VERSION}})
    nodePath = path.join(context.hnvmDir, 'node', VERSION)
    nodeBinDir = path.join(nodePath, 'bin')
    const {fingerprints} = binaryFingerprints({HNVM_DISTRUST_CERTS: 'false'})
    expect(fingerprints).toContain(AAA_CERTIFICATE_SERVICES)
    extra = fingerprints.find(fp => fp !== AAA_CERTIFICATE_SERVICES)
  })

  afterAll(() => {
    context.cleanup()
  })

  it('removes the roots when several invocations patch the same node at once', async () => {
    const env = {HNVM_DISTRUST_CERTS_EXTRA: opensslFingerprint(extra)}
    const results = await Promise.all([1, 2, 3, 4].map(() => runNode(env)))

    for (const result of results) {
      expect(result.status).toBe(0)
      expect(result.stdout).toContain(VERSION)
      expect(result.stderr).not.toContain('still trusts')
    }
    // The lock makes sure only one of them actually patched the binary
    expect(results.filter(result => result.stderr.includes('Removed 2 distrusted root certificate(s)'))).toHaveLength(1)
    expect(tmpFiles(nodeBinDir)).toEqual([])
    expect(fs.existsSync(path.join(nodePath, '.hnvm-distrust-lock'))).toBe(false)

    const {fingerprints, stderr} = binaryFingerprints(env)
    expect(stderr).not.toContain('distrusted')
    expect(fingerprints).not.toContain(AAA_CERTIFICATE_SERVICES)
    expect(fingerprints).not.toContain(extra)
    expect(fingerprints.length).toBeGreaterThan(100)
  })
})
