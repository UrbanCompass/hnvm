package install

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// NodeTarget returns the platform and arch strings used in Node's download file names. Unlike the
// bash shim, Linux arm64 (reported as aarch64 by uname) gets arm64 builds. Node only ships arm64
// darwin builds from v16.
func NodeTarget(goos, goarch, version string) (platform, arch string, err error) {
	switch goos {
	case "linux", "darwin":
		platform = goos
	default:
		return "", "", fmt.Errorf("OS Platform not supported")
	}

	arch = "x64"
	if goarch == "arm64" {
		major, _, _ := strings.Cut(version, ".")
		if n, err := strconv.Atoi(major); err == nil && n >= 16 {
			arch = "arm64"
		}
	}
	return platform, arch, nil
}

func hostNodeTarget(version string) (string, string, error) {
	return NodeTarget(runtime.GOOS, runtime.GOARCH, version)
}
