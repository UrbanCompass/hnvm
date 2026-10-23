const fs = require('fs')
const path = require('path')
const {createTestContext} = require('./utils.js')

jest.setTimeout(120_000)

const EMSIGN_ECC_G3 = '86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b'
const AAA_CERTIFICATE_SERVICES = 'd7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4'

// Prints the SHA-256 fingerprints of every root certificate bundled into the running node
const PRINT_ROOT_FINGERPRINTS = `
  require('tls').rootCertificates.map(pem => require('crypto').createHash('sha256')
    .update(Buffer.from(pem.replace(/-----[A-Z ]+-----|\\s/g, ''), 'base64')).digest('hex')).join(',')
`

describe('removing distrusted root certificates from node', () => {
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
      expect(fs.readdirSync(nodeBinDir).filter(file => file.endsWith('.hnvm-tmp'))).toEqual([])
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
    // Colon-separated, upper case, as printed by `openssl x509 -fingerprint -sha256`
    const formatted = extra.toUpperCase().match(/../g).join(':')

    const {fingerprints, stderr} = rootFingerprints({HNVM_DISTRUST_CERTS_EXTRA: formatted})

    expect(stderr).toContain(`Removed 1 distrusted root certificate(s) from node v${VERSION}`)
    expect(fingerprints).not.toContain(extra)
    expect(fs.readFileSync(markerFile, 'utf-8')).not.toEqual(markerBefore)
  })
})
