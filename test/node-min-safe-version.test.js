const fs = require('fs')
const path = require('path')
const os = require('os')
const childProcess = require('child_process')
const {createTestContext, describeUnlessManaged} = require('./utils.js')

jest.setTimeout(120_000)

const libDir = path.join(__dirname, '../lib/hnvm')

// Runs apply_min_safe_node in a bash shell for a pinned version, without downloading anything
// afterSource is bash run right after the library is sourced. By default it points the managed-preferences file at a
// path that does not exist, so a developer Mac that has the real MDM profile installed cannot change the results.
const NO_MANAGED_PREFS = 'managed_prefs_plist=/nonexistent/hnvm-test'

function floor(pinned, env = {}, afterSource = NO_MANAGED_PREFS) {
  const result = childProcess.spawnSync(
    'bash',
    [
      '-c',
      'set -e; script_dir="$1"; COMMAND_OUTPUT="&2"; source "$script_dir/min-safe-node.sh"; eval "$3"; node_ver="$2"; apply_min_safe_node; echo "RESULT=$node_ver"',
      'bash',
      libDir,
      pinned,
      afterSource,
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
      ['-c', 'script_dir="$1"; COMMAND_OUTPUT=/dev/null; source "$script_dir/min-safe-node.sh"; ' + NO_MANAGED_PREFS + '; node_ver=22.1.0; apply_min_safe_node', 'bash', libDir],
      {encoding: 'utf-8', env: {PATH: process.env.PATH}},
    )
    expect(result.stderr).toContain('using 22.23.4 instead')
  })

  it('still follows HNVM_OUTPUT_DESTINATION when HNVM_QUIET has pointed COMMAND_OUTPUT at /dev/null', () => {
    const dest = path.join(os.tmpdir(), `hnvm-warning-${process.pid}`)
    fs.writeFileSync(dest, '')
    const result = childProcess.spawnSync(
      'bash',
      ['-c', 'script_dir="$1"; COMMAND_OUTPUT=/dev/null; source "$script_dir/min-safe-node.sh"; ' + NO_MANAGED_PREFS + '; node_ver=22.1.0; apply_min_safe_node', 'bash', libDir],
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
      ['-c', 'set -e; script_dir="$1"; COMMAND_OUTPUT=/dev/null; source "$script_dir/min-safe-node.sh"; ' + NO_MANAGED_PREFS + '; node_ver=22.1.0; exec 2>&-; apply_min_safe_node; echo "RESULT=$node_ver"', 'bash', libDir],
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

// A stand-in for managed_pref, so precedence is tested on any platform (CI runs on Linux, where there is no `defaults`)
const stubManaged = (safe, major) =>
  `managed_pref() { case "$1" in HNVM_MIN_SAFE_NODE) echo "${safe}" ;; HNVM_MIN_SAFE_NODE_MAJOR) echo "${major}" ;; esac; }`

describe('managed (MDM) settings', () => {
  it('make a same-line bump happen even when the environment opts out', () => {
    const result = floor('22.1.0', {HNVM_MIN_SAFE_NODE: 'false'}, stubManaged('true', ''))
    expect(result.version).toBe('22.23.4')
    expect(result.stderr).toContain('using 22.23.4 instead (set by your organization)')
    expect(result.stderr).not.toContain('opt out')
  })

  it('can turn the same-line bump off, whatever the environment says', () => {
    const result = floor('22.1.0', {HNVM_MIN_SAFE_NODE: 'true'}, stubManaged('false', ''))
    expect(result.version).toBe('22.1.0')
    expect(result.stderr).toBe('')
  })

  it('move a pin with no fix across majors even when the environment does not ask for it', () => {
    const result = floor('18.12.0', {}, stubManaged('', 'true'))
    expect(result.version).toBe('24.22.1')
  })

  it('can forbid the cross-major move, whatever the environment says', () => {
    const result = floor('18.12.0', {HNVM_MIN_SAFE_NODE_MAJOR: 'true'}, stubManaged('', 'false'))
    expect(result.version).toBe('18.12.0')
    expect(result.stderr).toContain('no fix on its line. Move to 24.22.1 or newer.')
    expect(result.stderr).not.toContain('HNVM_MIN_SAFE_NODE_MAJOR')
  })

  it('leave the environment in charge for a key they do not set', () => {
    const result = floor('18.12.0', {HNVM_MIN_SAFE_NODE_MAJOR: 'true'}, stubManaged('false', ''))
    expect(result.version).toBe('24.22.1')
  })
})

const describeOnMac = process.platform === 'darwin' ? describe : describe.skip

describeOnMac('reading the managed preferences file with defaults', () => {
  let dir
  const plist = () => path.join(dir, 'com.compass.hnvm')
  const write = (key, type, value) => childProcess.execFileSync('/usr/bin/defaults', ['write', plist(), key, type, value])
  const read = key => {
    const out = childProcess.spawnSync(
      'bash',
      ['-c', 'script_dir="$1"; source "$script_dir/min-safe-node.sh"; managed_prefs_plist="$2"; managed_pref "$3"', 'bash', libDir, plist(), key],
      {encoding: 'utf-8', env: {PATH: process.env.PATH}},
    )
    return out.stdout.trim()
  }

  beforeAll(() => {
    dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hnvm-managed'))
    write('HNVM_MIN_SAFE_NODE', '-bool', 'false')
    write('HNVM_MIN_SAFE_NODE_MAJOR', '-string', 'TRUE')
    write('SOMETHING_ELSE', '-string', 'maybe')
  })

  afterAll(() => fs.rmSync(dir, {recursive: true, force: true}))

  it('reads a boolean', () => expect(read('HNVM_MIN_SAFE_NODE')).toBe('false'))
  it('reads a string, ignoring case', () => expect(read('HNVM_MIN_SAFE_NODE_MAJOR')).toBe('true'))
  it('prints nothing for a value that is not a boolean', () => expect(read('SOMETHING_ELSE')).toBe(''))
  it('prints nothing for a key that is not set', () => expect(read('NOT_THERE')).toBe(''))
  it('prints nothing when the file does not exist', () => {
    fs.rmSync(plist() + '.plist')
    expect(read('HNVM_MIN_SAFE_NODE')).toBe('')
  })
})

describeUnlessManaged('node pinned below a fixed release', () => {
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
