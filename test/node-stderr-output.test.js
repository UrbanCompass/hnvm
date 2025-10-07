const {createTestContext} = require('./utils.js')
const path = require('path')

jest.setTimeout(60_000)

describe('node stderr output behavior', () => {
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: '14.18.0'}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('should detect stderr as writable', () => {
    // Test what COMMAND_OUTPUT gets set to
    const configPath = path.join(__dirname, '../lib/hnvm/config.sh')
    const result = context.execFileSyncSeparateStreams('bash', [
      '-c',
      'source ' + configPath + ' && echo "COMMAND_OUTPUT=$COMMAND_OUTPUT"'
    ])
    
    console.log('COMMAND_OUTPUT detection result:', result.stdout)
    console.log('stderr:', result.stderr)
    
    expect(result.stdout).toMatch(/COMMAND_OUTPUT=\/dev\/(fd\/2|stderr)/)
  })

  it('should send HNVM messages to stderr by default', () => {
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['-p', '"Hello, World!"'])
    
    // Actual node output should be in stdout
    expect(result.stdout).toContain('Hello, World!')
    
    // HNVM messages should be in stderr
    expect(result.stderr).toMatch(/Using Hermetic NodeJS/)
    expect(result.stderr).toMatch(/Downloading/)
  })

  it('should not include HNVM messages in stdout by default', () => {
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['--version'])
    
    // stdout should only contain the node version, no HNVM messages
    expect(result.stdout).toMatch(/^v14\.18\.0\n$/)
    expect(result.stdout).not.toMatch(/Using Hermetic NodeJS/)
    expect(result.stdout).not.toMatch(/Downloading/)
  })

  it('should send "Resolved" messages to stderr', () => {
    context.createPackageJson({engines: {node: '>=14'}})
    
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['--version'])
    
    // stdout should only contain the node version
    expect(result.stdout).toMatch(/^v14\.18\.0\n$/)
    
    // "Resolved" message should be in stderr
    expect(result.stderr).toMatch(/Resolved.*14 to 14.18.0/)
  })

  it('should keep node command output in stdout', () => {
    const result = context.execFileSyncSeparateStreams(context.binaries.node, ['-e', 'console.log("test output")'])
    
    // Command output should be in stdout
    expect(result.stdout).toContain('test output')
    
    // HNVM messages should be in stderr
    expect(result.stderr).toMatch(/Using Hermetic NodeJS/)
  })
})
