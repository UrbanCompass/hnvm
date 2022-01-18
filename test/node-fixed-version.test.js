const fs = require('fs')
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

  it('should run without an error exit code', () => {
    const result = context.execFileSync(context.binaries.node, ['-p', '"Hello, World!"'])
    expect(result).toContain('Hello, World!')
  })

  it('should have downloaded the correct version', () => {
    const files = fs.readdirSync(context.hnvmDir + '/node')
    expect(files).toContain(VERSION)
  })

  it('should use the correct version when attempting to run node', () => {
    const result = context.execFileSync(context.binaries.node, ['--version'])
    expect(result).toContain(VERSION)
  })
})
