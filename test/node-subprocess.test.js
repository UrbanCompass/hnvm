const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('node with a fixed verison', () => {
  const VERSION = '16.13.0'
  const PNPM_VERSION = '6.0.0'
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: VERSION, pnpm: PNPM_VERSION}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('should fallback to using "/dev/null" if HNVM_OUTPUT_DESTINATION is a socket', () => {
    const hnvmProcess = context.execFileSyncWithSocketOutput(
      context.binaries.node,
      ['-p', '"Hello, World!"']
    )

    expect(hnvmProcess.stdout).toContain('Hello, World!')
    expect(hnvmProcess.stderr).toContain(
      "WARNING: Could not find a writable, non-socket stdout redirect target!"
    )
    expect(hnvmProcess.stderr).toContain("WARNING: Further HNVM output will be redirected to '/dev/null'")
  })

  it('[TIP-8901] should give only the pnpm version on stdout, when HNVM_OUTPUT_DESTINATION is a socket', () => {
    const hnvmProcess = context.execFileSyncWithSocketOutput(
      context.binaries.pnpm,
      ['--version']
    )

    expect(hnvmProcess.stdout).toContain('6.0.0')
    expect(hnvmProcess.stderr).toContain(
      "WARNING: Could not find a writable, non-socket stdout redirect target!"
    )
    expect(hnvmProcess.stderr).toContain("WARNING: Further HNVM output will be redirected to '/dev/null'")
  });
})
