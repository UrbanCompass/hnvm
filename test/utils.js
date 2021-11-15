const path = require('path')
const os = require('os')
const fs = require('fs')
const childProcess = require('child_process')

const hnvmBinDir = path.join(__dirname, '../bin')

function createTestContext() {
  let testDir
  let hnvmDir
  let cwdDir

  testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'hnvm'))
  hnvmDir = path.join(testDir, 'hnvm')
  cwdDir = path.join(testDir, 'client')

  fs.mkdirSync(hnvmDir)
  fs.mkdirSync(cwdDir)

  return {
    testDir,
    hnvmDir,
    cwdDir,
    binaries: {
      node: path.join(hnvmBinDir, 'node'),
      npm: path.join(hnvmBinDir, 'npm'),
      pnpm: path.join(hnvmBinDir, 'pnpm'),
    },
    createPackageJson(json) {
      fs.writeFileSync(path.join(cwdDir, 'package.json'), JSON.stringify(json))
    },
    cleanup() {
      fs.rmSync(testDir, {recursive: true, force: true})
    },
    execFileSync(file, args) {
      return childProcess.execFileSync(file, args, {
        encoding: 'utf-8',
        env: {HNVM_PATH: hnvmDir},
        cwd: cwdDir,
      })
    },
  }
}

module.exports = {createTestContext}
