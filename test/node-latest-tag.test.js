const fs = require('fs')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('node with a tag (latest)', () => {
  const VERSION = 'latest'
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

  it('should have downloaded a version of node', () => {
    const files = fs.readdirSync(context.hnvmDir + '/node')
    expect(files).toHaveLength(1)
    expect(files[0]).toMatch(/\d+\.\d+\.\d+/)
  })

  it('should run node successfully', () => {
    context.execFileSync(context.binaries.node, ['--version'])
  })
})
