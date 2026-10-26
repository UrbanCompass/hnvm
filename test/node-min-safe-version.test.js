const fs = require('fs')
const path = require('path')
const os = require('os')
const childProcess = require('child_process')
const {createTestContext} = require('./utils.js')

jest.setTimeout(120_000)

const libDir = path.join(__dirname, '../lib/hnvm')

// Runs apply_min_safe_node in a bash shell for a pinned version, without downloading anything
function floor(pinned, env = {}) {
  const result = childProcess.spawnSync(
    'bash',
    [
      '-c',
      'set -e; script_dir="$1"; COMMAND_OUTPUT="&2"; source "$script_dir/min-safe-node.sh"; node_ver="$2"; apply_min_safe_node; echo "RESULT=$node_ver"',
      'bash',
      libDir,
      pinned,
    ],
    {encoding: 'utf-8', env: {PATH: process.env.PATH, ...env}},
  )
  return {
    version: /RESULT=(.*)/.exec(result.stdout)[1],
    stderr: result.stderr,
    status: result.status,
  }
}

describe('minimum safe node version rules', () => {
  it.each([
    ['22.1.0', '22.23.4'],
    ['22.23.3', '22.23.4'],
    ['24.0.0', '24.22.1'],
    ['26.5.0', '26.12.1'],
  ])('raises %s to %s on the same line', (pinned, expected) => {
    const result = floor(pinned)
    expect(result.version).toBe(expected)
    expect(result.stderr).toContain(`node ${pinned} has known security problems; using ${expected} instead`)
  })

  it.each(['22.23.4', '22.99.0', '24.22.1', '26.12.1', '27.0.0', '30.1.2'])(
    'leaves %s alone, without a warning',
    pinned => {
      const result = floor(pinned)
      expect(result.version).toBe(pinned)
      expect(result.stderr).toBe('')
    },
  )

  it('compares versions numerically, not as text', () => {
    expect(floor('22.9.0').version).toBe('22.23.4')
    expect(floor('22.100.0').version).toBe('22.100.0')
  })

  it.each(['23.5.0', '25.1.0', '20.11.0', '18.12.0', '16.13.0', '9.0.0'])(
    'warns but keeps %s, which has no fix on its line, by default',
    pinned => {
      const result = floor(pinned)
      expect(result.version).toBe(pinned)
      expect(result.stderr).toContain('no fix on its line')
      expect(result.stderr).toContain('HNVM_MIN_SAFE_NODE_MAJOR=true')
    },
  )

  it.each([
    ['23.5.0', '24.22.1'],
    ['25.1.0', '26.12.1'],
    ['20.11.0', '24.22.1'],
    ['18.12.0', '24.22.1'],
    ['9.0.0', '24.22.1'],
  ])('moves %s to %s across majors when HNVM_MIN_SAFE_NODE_MAJOR=true', (pinned, expected) => {
    expect(floor(pinned, {HNVM_MIN_SAFE_NODE_MAJOR: 'true'}).version).toBe(expected)
  })

  it('can be turned off for a same-line bump', () => {
    const result = floor('22.1.0', {HNVM_MIN_SAFE_NODE: 'false'})
    expect(result.version).toBe('22.1.0')
    expect(result.stderr).toBe('')
  })

  it('ignores anything that is not an exact version', () => {
    expect(floor('latest').version).toBe('latest')
    expect(floor('>=22').version).toBe('>=22')
  })

  it('ignores HNVM_QUIET, so a security notice is never hidden', () => {
    const result = childProcess.spawnSync(
      'bash',
      ['-c', 'script_dir="$1"; COMMAND_OUTPUT=/dev/null; source "$script_dir/min-safe-node.sh"; node_ver=22.1.0; apply_min_safe_node', 'bash', libDir],
      {encoding: 'utf-8', env: {PATH: process.env.PATH}},
    )
    expect(result.stderr).toContain('using 22.23.4 instead')
  })

  it('still follows HNVM_OUTPUT_DESTINATION when HNVM_QUIET has pointed COMMAND_OUTPUT at /dev/null', () => {
    const dest = path.join(os.tmpdir(), `hnvm-warning-${process.pid}`)
    fs.writeFileSync(dest, '')
    const result = childProcess.spawnSync(
      'bash',
      ['-c', 'script_dir="$1"; COMMAND_OUTPUT=/dev/null; source "$script_dir/min-safe-node.sh"; node_ver=22.1.0; apply_min_safe_node', 'bash', libDir],
      {encoding: 'utf-8', env: {PATH: process.env.PATH, HNVM_OUTPUT_DESTINATION: dest, HNVM_QUIET: 'true'}},
    )
    const written = fs.readFileSync(dest, 'utf-8')
    fs.rmSync(dest)
    expect(written).toContain('using 22.23.4 instead')
    expect(result.stderr).toBe('')
  })

  it('does not stop hnvm when the warning cannot be written (stderr closed)', () => {
    const result = childProcess.spawnSync(
      'bash',
      ['-c', 'set -e; script_dir="$1"; COMMAND_OUTPUT=/dev/null; source "$script_dir/min-safe-node.sh"; node_ver=22.1.0; exec 2>&-; apply_min_safe_node; echo "RESULT=$node_ver"', 'bash', libDir],
      {encoding: 'utf-8', env: {PATH: process.env.PATH}},
    )
    expect(result.status).toBe(0)
    expect(result.stdout).toContain('RESULT=22.23.4')
  })

  it('has rules for every line of a fixed release', () => {
    const rules = fs
      .readFileSync(path.join(libDir, 'min-safe-node.txt'), 'utf-8')
      .split('\n')
      .filter(line => line.trim() && !line.startsWith('#'))
    expect(rules).toEqual(['<22 24.22.1', '22 22.23.4', '23 24.22.1', '24 24.22.1', '25 26.12.1', '26 26.12.1'])
  })
})

describe('node pinned below a fixed release', () => {
  let context

  beforeAll(() => {
    context = createTestContext()
    context.createPackageJson({engines: {node: '22.1.0'}})
  })

  afterAll(() => {
    context.cleanup()
  })

  it('runs the fixed release of its line instead', () => {
    const result = context.execFileSync(context.binaries.node, ['--version'])
    expect(result).toContain('using 22.23.4 instead')
    expect(result).toContain('v22.23.4')
    expect(fs.readdirSync(context.hnvmDir + '/node')).toEqual(['22.23.4'])
  })

  it('runs the pinned version when opted out through .hnvmrc', () => {
    fs.writeFileSync(path.join(context.cwdDir, '.hnvmrc'), 'HNVM_MIN_SAFE_NODE=false\n')
    const result = context.execFileSync(context.binaries.node, ['--version'])
    expect(result).toContain('v22.1.0')
    expect(result).not.toContain('known security problems')
  })
})
