const fs = require('fs')
const semver = require('./semver.js')

function isValidSemverRange(range) {
  try {
    new semver.Range(range)
    return true
  } catch (_) {
    return false
  }
}

const {desiredVersionRange, availableVersionsColonDelimited, npmPackageInfo} = JSON.parse(
  fs.readFileSync(0, 'utf-8'),
)

let matchedVersion
if (availableVersionsColonDelimited) {
  const availableVersions = availableVersionsColonDelimited.split(':').filter(Boolean)
  const availableMatchingVersion = availableVersions.find(version =>
    semver.satisfies(version, desiredVersionRange),
  )

  matchedVersion = availableMatchingVersion
} else if (npmPackageInfo) {
  if (isValidSemverRange(desiredVersionRange)) {
    const existingVersions = Object.keys(npmPackageInfo.versions)
    const latestVersions = existingVersions.sort((a, b) => (a === b ? 0 : semver.gt(a, b) ? -1 : 1))

    const matchingVersions = latestVersions.filter(version =>
      semver.satisfies(version, desiredVersionRange),
    )

    matchedVersion = matchingVersions[0]
  } else {
    const tagInfo = npmPackageInfo['dist-tags'][desiredVersionRange]
    if (!tagInfo) {
      process.stderr.write(
        `\u001b[0;31mERROR\u001b[0m: "${desiredVersionRange}" was not a valid semver range, nor was it a dist-tag of ${npmPackageInfo.name}`,
      )
      process.exit(1)
    }
  }
} else {
  process.stderr.write(
    'Must specify one of `availableVersionsColonDelimited` or `existingVersions`\n',
  )
  process.exit(1)
}

if (matchedVersion) {
  process.stdout.write(`${matchedVersion}\n`)
} else {
  process.stderr.write(`Failed to find a matching version for "${desiredVersionRange}"!\n`)
  process.exit(1)
}
