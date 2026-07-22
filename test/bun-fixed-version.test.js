const fs = require('fs')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('bun with a fixed version', () => {
  const VERSION = '1.3.14'
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {bun: VERSION}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('should run without an error exit code', () => {
    const result = context.execFileSync(context.binaries.bun, ['--eval', 'console.log("Hello, World!")'])
    expect(result).toContain('Hello, World!')
  })

  it('should have downloaded the correct version', () => {
    const files = fs.readdirSync(context.hnvmDir + '/bun')
    expect(files).toContain(VERSION)
  })

  it('should use the correct version when attempting to run bun', () => {
    const result = context.execFileSync(context.binaries.bun, ['--version'])
    expect(result).toContain(VERSION)
  })

  it('should not download node for a bun invocation', () => {
    expect(fs.existsSync(context.hnvmDir + '/node')).toBe(false)
  })
})
