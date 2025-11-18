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

  it('should fail with 404 when trying to download a non-existent variant', () => {
    const outputFile = path.join(context.testDir, 'stdout')
    fs.writeFileSync(outputFile, '')

    // Use a variant that doesn't exist to verify the URL construction
    const nonExistentVariant = 'nonexistent-variant-test'
    
    try {
      childProcess.execFileSync(context.binaries.node, ['--version'], {
        encoding: 'utf-8',
        env: {
          HNVM_PATH: context.hnvmDir, 
          HNVM_OUTPUT_DESTINATION: outputFile,
          HNVM_NODE_VARIANT: nonExistentVariant,
          PATH: process.env.PATH
        },
        cwd: context.cwdDir,
        stdio: 'pipe'
      })
      // If we reach here, the test should fail because we expect an error
      throw new Error('Expected execFileSync to throw an error for non-existent variant')
    } catch (error) {
      // The curl command should fail with 404 when trying to download a non-existent variant
      // This confirms the variant is being appended to the URL
      expect(error.status).not.toBe(0)
      expect(error.stderr.toString()).toContain('404')
    }
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
