const childProcess = require('child_process')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('bun URL validation', () => {
  it('should provide a clear error message for an invalid bun version', () => {
    const invalidVersion = '999.999.999'
    const context = createTestContext()
    context.createPackageJson({engines: {bun: invalidVersion}})

    try {
      const result = childProcess.spawnSync(context.binaries.bun, ['--version'], {
        encoding: 'utf-8',
        env: {
          HNVM_PATH: context.hnvmDir,
          PATH: process.env.PATH,
        },
        cwd: context.cwdDir,
      })

      expect(result.status).not.toBe(0)
      expect(result.stderr).toContain('URL validation failed')
      expect(result.stderr).toContain('The requested package/version may not exist')
    } finally {
      context.cleanup()
    }
  })

  it('should fail when an invalid variant is requested', () => {
    const VERSION = '1.3.14'
    const nonExistentVariant = 'invalid-variant-xyz'
    const context = createTestContext()
    context.createPackageJson({engines: {bun: VERSION}})

    try {
      const result = childProcess.spawnSync(context.binaries.bun, ['--version'], {
        encoding: 'utf-8',
        env: {
          HNVM_PATH: context.hnvmDir,
          HNVM_BUN_VARIANT: nonExistentVariant,
          PATH: process.env.PATH,
        },
        cwd: context.cwdDir,
      })

      expect(result.status).not.toBe(0)
      expect(result.stderr).toContain('URL validation failed')
      expect(result.stderr).toContain(`HNVM_BUN_VARIANT='${nonExistentVariant}'`)
    } finally {
      context.cleanup()
    }
  })
})
