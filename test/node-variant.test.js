const fs = require('fs')
const path = require('path')
const childProcess = require('child_process')
const {createTestContext} = require('./utils.js')

jest.setTimeout(60_000)

describe('node with a variant', () => {
  const VERSION = '16.13.0'
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: VERSION}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('should fail validation when trying to use a non-existent variant', () => {
    // Use a variant that doesn't exist to verify the URL construction and validation
    const nonExistentVariant = 'nonexistent-variant-test'
    
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
    
    // Should fail during URL validation
    expect(result.status).not.toBe(0)
    
    // Error messages go to stderr
    expect(result.stderr).toContain('URL validation failed')
    
    // Should mention the variant in the error message
    expect(result.stderr).toContain(`HNVM_NODE_VARIANT='${nonExistentVariant}'`)
  })

  it('should work without variant when HNVM_NODE_VARIANT is not set', () => {
    const outputFile = path.join(context.testDir, 'stdout')
    fs.writeFileSync(outputFile, '')

    // Clean up any previously downloaded versions to force a fresh download
    const nodeVersionPath = path.join(context.hnvmDir, 'node', VERSION)
    if (fs.existsSync(nodeVersionPath)) {
      fs.rmSync(nodeVersionPath, {recursive: true, force: true})
    }

    // Execute node without HNVM_NODE_VARIANT
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
    
    // Verify it downloaded successfully
    expect(output).toContain(VERSION)
  })

  it('should work with empty variant when HNVM_NODE_VARIANT is empty string', () => {
    const outputFile = path.join(context.testDir, 'stdout')
    fs.writeFileSync(outputFile, '')

    // Clean up any previously downloaded versions to force a fresh download
    const nodeVersionPath = path.join(context.hnvmDir, 'node', VERSION)
    if (fs.existsSync(nodeVersionPath)) {
      fs.rmSync(nodeVersionPath, {recursive: true, force: true})
    }

    // Execute node with HNVM_NODE_VARIANT set to empty string (should behave same as not set)
    const result = childProcess.execFileSync(context.binaries.node, ['--version'], {
      encoding: 'utf-8',
      env: {
        HNVM_PATH: context.hnvmDir, 
        HNVM_OUTPUT_DESTINATION: outputFile,
        HNVM_NODE_VARIANT: '',
        PATH: process.env.PATH
      },
      cwd: context.cwdDir,
    })

    const output = fs.readFileSync(outputFile, 'utf-8') + result
    
    // Verify it downloaded successfully with standard (non-variant) URL
    expect(output).toContain(VERSION)
  })
})
