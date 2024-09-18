const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('node with a fixed verison', () => {
  const VERSION = '16.13.0'
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: VERSION}})
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
})
