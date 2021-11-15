const fs = require('fs')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('node with a semver range', () => {
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: '14.18.0'}})
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
    expect(files).toEqual(['14.18.0'])
  })

  it('should update the requirement to a semver range', () => {
    context.createPackageJson({engines: {node: '>=14'}})
  })

  it('should run without an error exit code', () => {
    const result = context.execFileSync(context.binaries.node, ['-p', '"Hello, World!"'])
    expect(result).toContain('Hello, World!')
    expect(result).toMatch(/Resolved.*14 to 14.18.0/)
  })

  it('should use the correct version when attempting to run node', () => {
    const result = context.execFileSync(context.binaries.node, ['--version'])
    expect(result).toContain('14.18.0')
  })

  it('should not have downloaded a new version', () => {
    const files = fs.readdirSync(context.hnvmDir + '/node')
    expect(files).toEqual(['14.18.0'])
  })

  it('should update the requirement to a non-matching semver range', () => {
    context.createPackageJson({engines: {node: '>=16'}})
  })

  it('should run without an error exit code', () => {
    const result = context.execFileSync(context.binaries.node, ['-p', '"Hello, World!"'])
    expect(result).toContain('Hello, World!')
  })

  it('should have downloaded a new version', () => {
    const files = fs.readdirSync(context.hnvmDir + '/node')
    expect(files).toHaveLength(2)
    expect(files).toContain('14.18.0')
  })
})
