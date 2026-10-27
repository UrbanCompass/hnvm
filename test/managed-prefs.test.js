const fs = require('fs')
const path = require('path')
const os = require('os')
const childProcess = require('child_process')

const libDir = path.join(__dirname, '../lib/hnvm')

// Runs bash with managed-prefs.sh sourced. stubs is bash run right after sourcing, to stand in for the profile.
function run(script, env = {}, stubs = '') {
  const result = childProcess.spawnSync(
    'bash',
    ['-c', 'set -e; script_dir="$1"; source "$script_dir/managed-prefs.sh"; eval "$2"; eval "$3"', 'bash', libDir, stubs, script],
    {encoding: 'utf-8', env: {PATH: process.env.PATH, ...env}},
  )
  expect(result.stderr).toBe('')
  expect(result.status).toBe(0)
  return result.stdout
}

// A stand-in for managed_pref_raw, so parsing and precedence are tested on any platform (CI runs on Linux, where there is no
// `defaults`). values maps a key to what `defaults read` would print for it; any other key is not set.
const stubRaw = values =>
  'managed_pref_raw() { case "$1" in ' +
  Object.entries(values)
    .map(([key, value]) => `${key}) printf '%b\\n' ${JSON.stringify(value)} ;;`)
    .join(' ') +
  ' *) return 1 ;; esac; }'

const PRINT_DISTRUST =
  'apply_managed_distrust_certs; ' +
  'echo "certs=$HNVM_DISTRUST_CERTS/$distrust_certs_managed extra=$HNVM_DISTRUST_CERTS_EXTRA/$distrust_certs_extra_managed ' +
  'strict=$HNVM_DISTRUST_CERTS_STRICT/$distrust_certs_strict_managed"'

const FP_A = 'a'.repeat(64)
const FP_B = 'b'.repeat(64)

describe('managed (MDM) distrust settings', () => {
  it('leave the environment in charge when the profile sets none of them', () => {
    const env = {HNVM_DISTRUST_CERTS: 'false', HNVM_DISTRUST_CERTS_EXTRA: FP_A, HNVM_DISTRUST_CERTS_STRICT: 'true'}
    expect(run(PRINT_DISTRUST, env, stubRaw({})).trim()).toBe(`certs=false/ extra=${FP_A}/ strict=true/`)
  })

  it('win over the environment for every key the profile sets', () => {
    const env = {HNVM_DISTRUST_CERTS: 'false', HNVM_DISTRUST_CERTS_EXTRA: FP_A, HNVM_DISTRUST_CERTS_STRICT: 'false'}
    const stubs = stubRaw({HNVM_DISTRUST_CERTS: '1', HNVM_DISTRUST_CERTS_EXTRA: FP_B, HNVM_DISTRUST_CERTS_STRICT: 'YES'})
    expect(run(PRINT_DISTRUST, env, stubs).trim()).toBe(`certs=true/true extra=${FP_B}/true strict=true/true`)
  })

  it('can turn distrusting off, whatever the environment says', () => {
    const stubs = stubRaw({HNVM_DISTRUST_CERTS: '0'})
    expect(run(PRINT_DISTRUST, {HNVM_DISTRUST_CERTS: 'true'}, stubs)).toContain('certs=false/true')
  })

  it('ignore a value that is not a boolean', () => {
    const stubs = stubRaw({HNVM_DISTRUST_CERTS: 'maybe', HNVM_DISTRUST_CERTS_STRICT: 'maybe'})
    expect(run(PRINT_DISTRUST, {HNVM_DISTRUST_CERTS: 'false'}, stubs)).toContain('certs=false/ ')
  })

  it('clear the extra fingerprints with an empty list', () => {
    for (const empty of ['(\n)', '']) {
      const stubs = stubRaw({HNVM_DISTRUST_CERTS_EXTRA: empty})
      expect(run(PRINT_DISTRUST, {HNVM_DISTRUST_CERTS_EXTRA: FP_A}, stubs)).toContain('extra=/true')
    }
  })

  it('leave the environment in charge for a key they do not set', () => {
    const stubs = stubRaw({HNVM_DISTRUST_CERTS_STRICT: 'true'})
    expect(run(PRINT_DISTRUST, {HNVM_DISTRUST_CERTS_EXTRA: FP_A}, stubs).trim()).toBe(
      `certs=/ extra=${FP_A}/ strict=true/true`,
    )
  })
})

describe('managed_pref_list', () => {
  const list = raw => run('managed_pref_list KEY', {}, stubRaw({KEY: raw})).trim()

  it('passes a comma-separated string through', () => expect(list(`${FP_A},${FP_B}`)).toBe(`${FP_A},${FP_B}`))

  it('joins an array as printed by defaults', () => {
    expect(list(`(\n    ${FP_A},\n    "sha256 Fingerprint=${FP_B}"\n)`)).toBe(`${FP_A},sha256 Fingerprint=${FP_B}`)
  })

  it('prints nothing for an empty array, but reports it as set', () => {
    expect(run('managed_pref_list KEY && echo "[set]"', {}, stubRaw({KEY: '(\n)'})).trim()).toBe('[set]')
  })

  it('prints nothing for a key that is not set, and reports it as unset', () => {
    expect(run('managed_pref_list KEY || echo "[unset]"', {}, stubRaw({})).trim()).toBe('[unset]')
  })
})

const describeOnMac = process.platform === 'darwin' ? describe : describe.skip

describeOnMac('reading managed lists with defaults', () => {
  let dir
  const plist = () => path.join(dir, 'com.compass.hnvm')
  const read = key =>
    run(`managed_prefs_plist=${JSON.stringify(plist())}; managed_pref_list ${key} || echo "[unset]"`).trim()

  beforeAll(() => {
    dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hnvm-managed'))
    childProcess.execFileSync('/usr/bin/defaults', ['write', plist(), 'AS_ARRAY', '-array', FP_A, `sha256 Fingerprint=${FP_B}`])
    childProcess.execFileSync('/usr/bin/defaults', ['write', plist(), 'AS_STRING', '-string', `${FP_A},${FP_B}`])
    childProcess.execFileSync('/usr/bin/defaults', ['write', plist(), 'EMPTY_ARRAY', '-array'])
  })

  afterAll(() => fs.rmSync(dir, {recursive: true, force: true}))

  it('reads an array', () => expect(read('AS_ARRAY')).toBe(`${FP_A},sha256 Fingerprint=${FP_B}`))
  it('reads a string', () => expect(read('AS_STRING')).toBe(`${FP_A},${FP_B}`))
  it('reads an empty array as set', () => expect(read('EMPTY_ARRAY')).toBe(''))
  it('reports a key that is not set', () => expect(read('NOT_THERE')).toBe('[unset]'))
})
