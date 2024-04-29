const path = require('path')
const os = require('os')
const fs = require('fs')
const net = require('net')
const childProcess = require('child_process')

const hnvmBinDir = path.join(__dirname, '../bin')

function createTestContext() {
  let testDir
  let hnvmDir
  let cwdDir

  // Socket specific stuff
  let testStdoutServer
  let testStdoutSocket
  let stdoutServerConnections = []

  testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'hnvm'))
  hnvmDir = path.join(testDir, 'hnvm')
  cwdDir = path.join(testDir, 'client')

  fs.mkdirSync(hnvmDir)
  fs.mkdirSync(cwdDir)

  // https://gist.github.com/Xaekai/e1f711cb0ad865deafc11185641c632a
  testStdoutSocket = path.join(testDir, 'hnvm-jest.test.sock');
  testStdoutServer = net.createServer((inboundClient) => {
    stdoutServerConnections.push(inboundClient);

    inboundClient.on('end', () => {
      console.log('Client disconnected.');
    });

    inboundClient.on('data', (msg) => {
      const str = msg.toString();
      console.log('Server:: got client message ' + str);
      inboundClient.write(str);
    })

    // when a client first connects, ALWAYS write back to them
    inboundClient.write('client has connd to server')
  })
  .on('close', () => {
    for (let i = 0; i < stdoutServerConnections.length; i++) {
      const stdoutSocket = stdoutServerConnections[i];
      stdoutSocket.destroy();
    }
  })
  .listen(testStdoutSocket)

  return {
    testDir,
    hnvmDir,
    cwdDir,
    testStdoutServer,
    testStdoutSocket,
    binaries: {
      node: path.join(hnvmBinDir, 'node'),
      npm: path.join(hnvmBinDir, 'npm'),
      pnpm: path.join(hnvmBinDir, 'pnpm'),
    },
    createPackageJson(json) {
      fs.writeFileSync(path.join(cwdDir, 'package.json'), JSON.stringify(json))
    },
    cleanup() {
      // close() will close the server bound to the socket file,
      // and terminate any previously made connections
      testStdoutServer.close()
      fs.rmSync(testDir, {recursive: true, force: true})
    },
    execFileSync(file, args) {
      const outputFile = path.join(testDir, 'stdout')
      fs.writeFileSync(outputFile, '')

      const stdout = childProcess.execFileSync(file, args, {
        encoding: 'utf-8',
        env: {HNVM_PATH: hnvmDir, HNVM_OUTPUT_DESTINATION: outputFile},
        cwd: cwdDir,
      })

      return fs.readFileSync(outputFile, 'utf-8') + stdout
    },
    execFileSyncWithSocketOutput(file, args) {
      // Provide a socket as HNVM_OUTPUT_DESTINATION
      // Used for tests that want to test behavior when the redirect target is a socket
      const stdout = childProcess.execFileSync(file, args, {
        encoding: 'utf-8',
        env: {HNVM_PATH: hnvmDir, HNVM_OUTPUT_DESTINATION: testStdoutSocket},
        cwd: cwdDir,
      })

      return stdout
    },
  }
}

module.exports = {createTestContext}
