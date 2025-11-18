const fs = require('fs')
const path = require('path')
const childProcess = require('child_process')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('URL validation', () => {
  const VERSION = '16.13.0'
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: VERSION}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('should validate URL before downloading and provide helpful error message on failure', () => {
    // Use a variant that doesn't exist to trigger validation failure
    const nonExistentVariant = 'invalid-variant-xyz'
    
    try {
      // Use spawnSync to capture stderr (where error messages go)
      const result = childProcess.spawnSync(context.binaries.node, ['--version'], {
        encoding: 'utf-8',
        env: {
          HNVM_PATH: context.hnvmDir,
          HNVM_NODE_VARIANT: nonExistentVariant,
          PATH: process.env.PATH
        },
        cwd: context.cwdDir,
      })
      
      // Should fail with validation error
      expect(result.status).not.toBe(0)
      
      // Error messages go to stderr
      expect(result.stderr).toContain('URL validation failed')
      
      // Should provide helpful context about the variant
      expect(result.stderr).toContain(`HNVM_NODE_VARIANT='${nonExistentVariant}'`)
      expect(result.stderr).toContain('This variant may not be available')
    } catch (error) {
      throw new Error('Test setup failed: ' + error.message)
    }
  })

  it('should silently validate and download when URL is valid', () => {
    const outputFile = path.join(context.testDir, 'stdout')
    fs.writeFileSync(outputFile, '')

    // Clean up any previously downloaded versions to force a fresh download
    const nodeVersionPath = path.join(context.hnvmDir, 'node', VERSION)
    if (fs.existsSync(nodeVersionPath)) {
      fs.rmSync(nodeVersionPath, {recursive: true, force: true})
    }

    // Execute node without variant (should validate and download successfully)
    const result = childProcess.execFileSync(context.binaries.node, ['--version'], {
      encoding: 'utf-8',
      env: {
        HNVM_PATH: context.hnvmDir, 
        HNVM_OUTPUT_DESTINATION: outputFile,
        PATH: process.env.PATH
      },
      cwd: context.cwdDir,
    })

    const output = fs.readFileSync(outputFile, 'utf-8') + result
    
    // Should NOT show validation messages (only on failure)
    expect(output).not.toContain('Validating')
    expect(output).not.toContain('URL is valid')
    expect(output).not.toContain('URL validation failed')
    
    // Should proceed with download
    expect(output).toContain('Downloading node')
    expect(output).toContain(VERSION)
  })

  it('should skip validation when HNVM_SKIP_URL_VALIDATION is true', () => {
    const outputFile = path.join(context.testDir, 'stdout')
    fs.writeFileSync(outputFile, '')

    // Clean up any previously downloaded versions
    const nodeVersionPath = path.join(context.hnvmDir, 'node', VERSION)
    if (fs.existsSync(nodeVersionPath)) {
      fs.rmSync(nodeVersionPath, {recursive: true, force: true})
    }

    // Execute with HNVM_SKIP_URL_VALIDATION=true
    const result = childProcess.execFileSync(context.binaries.node, ['--version'], {
      encoding: 'utf-8',
      env: {
        HNVM_PATH: context.hnvmDir, 
        HNVM_OUTPUT_DESTINATION: outputFile,
        HNVM_SKIP_URL_VALIDATION: 'true',
        PATH: process.env.PATH
      },
      cwd: context.cwdDir,
    })

    const output = fs.readFileSync(outputFile, 'utf-8') + result
    
    // Should NOT show any validation messages
    expect(output).not.toContain('Validating')
    expect(output).not.toContain('URL validation failed')
    
    // Should still download successfully
    expect(output).toContain('Downloading node')
    expect(output).toContain(VERSION)
  })

  it('should provide clear error message for invalid node version', () => {
    const invalidVersion = '999.999.999'
    
    // Create a separate test context with invalid version
    const invalidContext = createTestContext()
    invalidContext.createPackageJson({engines: {node: invalidVersion}})
    
    try {
      // Use spawnSync to capture stderr (where error messages go)
      const result = childProcess.spawnSync(invalidContext.binaries.node, ['--version'], {
        encoding: 'utf-8',
        env: {
          HNVM_PATH: invalidContext.hnvmDir,
          PATH: process.env.PATH
        },
        cwd: invalidContext.cwdDir,
      })
      
      // Should fail with validation error
      expect(result.status).not.toBe(0)
      
      // Error messages go to stderr
      expect(result.stderr).toContain('URL validation failed')
      expect(result.stderr).toContain('The requested package/version may not exist')
    } finally {
      invalidContext.cleanup()
    }
  })

  it('should output only download messages to stderr on success, not stdout', () => {
    // Clean up any previously downloaded versions to force a fresh download
    const nodeVersionPath = path.join(context.hnvmDir, 'node', VERSION)
    if (fs.existsSync(nodeVersionPath)) {
      fs.rmSync(nodeVersionPath, {recursive: true, force: true})
    }

    // Execute node WITHOUT HNVM_OUTPUT_DESTINATION to test default stderr behavior
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['--version'])
    
    // Download messages should appear in stderr
    expect(result.stderr).toContain('Downloading node')
    
    // Validation success messages should NOT appear (silent on success)
    expect(result.stderr).not.toContain('Validating')
    expect(result.stderr).not.toContain('URL is valid')
    
    // Only the actual node --version output should be in stdout
    expect(result.stdout).toContain(`v${VERSION}`)
    
    // HNVM messages should NOT be in stdout
    expect(result.stdout).not.toContain('Downloading node')
    expect(result.stdout).not.toContain('Validating')
  })

  it('should output validation errors to stderr when validation fails', () => {
    const nonExistentVariant = 'invalid-test-variant'
    
    // Create a test context with invalid variant
    const variantContext = createTestContext()
    variantContext.createPackageJson({engines: {node: VERSION}})
    
    // Execute with an invalid variant WITHOUT HNVM_OUTPUT_DESTINATION
    try {
      const result = childProcess.spawnSync(variantContext.binaries.node, ['--version'], {
        encoding: 'utf-8',
        env: {
          HNVM_PATH: variantContext.hnvmDir,
          HNVM_NODE_VARIANT: nonExistentVariant,
          PATH: process.env.PATH
        },
        cwd: variantContext.cwdDir,
      })
      
      // Should have failed
      expect(result.status).not.toBe(0)
      
      // All error messages should be in stderr
      expect(result.stderr).toContain('URL validation failed')
      expect(result.stderr).toContain(`HNVM_NODE_VARIANT='${nonExistentVariant}'`)
      
      // Nothing about validation should leak to stdout
      expect(result.stdout).not.toContain('URL validation failed')
      expect(result.stdout).not.toContain('HNVM_NODE_VARIANT')
    } finally {
      variantContext.cleanup()
    }
  })
})
