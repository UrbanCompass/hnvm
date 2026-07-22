const fs = require('fs')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('bun with a semver range', () => {
  const RANGE = '^1'
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {bun: RANGE}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('should resolve the range and download a matching version', () => {
    context.execFileSync(context.binaries.bun, ['--version'])

    const files = fs.readdirSync(context.hnvmDir + '/bun')
    expect(files).toHaveLength(1)
    expect(files[0]).toMatch(/^1\.\d+\.\d+$/)
  })

  it('should run bun successfully', () => {
    const result = context.execFileSync(context.binaries.bun, ['--eval', 'console.log("Hello, World!")'])
    expect(result).toContain('Hello, World!')
  })
})
