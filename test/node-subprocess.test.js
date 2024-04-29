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
    const result = context.execFileSyncWithSocketOutput(
      context.binaries.node,
      ['-p', '"Hello, World!"']
    )

    expect(result).toContain('Hello, World!')
    expect(result).toContain(
      "WARNING: Could not find a writable, non-socket stdout redirect target!"
    )
    expect(result).toContain("WARNING: Further HNVM output will be redirected to '/dev/null'")
  })
})
