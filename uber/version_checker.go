package uber

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
)

const (
	REMOTE_URL = "ssh://gitolite@code.uber.internal/devexp/dep"
)

var CmdExecutor ExecutorInterface

func init() {
	CmdExecutor = new(CommandExecutor)
}

type VersionInfo struct {
	IsLatest      bool
	LatestVersion string
}

// IsLatestVersion calls ls-remote on the dep remote repo to find the latest semver tag
// and then compares it with the version passed in. It then returns if the version
// passed in is the latest version, and also what the latest version currently is
func IsLatestVersion(version string) (VersionInfo, error) {
	curSemVer, err := semver.NewVersion(version)
	if err != nil {
		return VersionInfo{}, err
	}
	latestSemVer, err := getLatestVersion()
	if err != nil {
		return VersionInfo{}, err
	}
	if curSemVer.LessThan(*latestSemVer) {
		return VersionInfo{LatestVersion: latestSemVer.String()}, nil
	} else if curSemVer.GreaterThan(*latestSemVer) {
		return VersionInfo{IsLatest: true, LatestVersion: curSemVer.String()},
			fmt.Errorf("current version %v later than latest %v", curSemVer.String(), latestSemVer.String())
	}
	return VersionInfo{IsLatest: true, LatestVersion: latestSemVer.String()}, nil
}

func getLatestVersion() (*semver.Version, error) {
	stdout, stderr, err := CmdExecutor.ExecCommand("git", time.Duration(30*time.Second),
		false, append([]string{"GIT_ASKPASS=", "GIT_TERMINAL_PROMPT=0"}, os.Environ()...), "ls-remote", "--tags", REMOTE_URL)

	if err != nil {
		return nil, errors.Wrap(err, stderr)
	}

	all := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(all) == 1 && len(all[0]) == 0 {
		return nil, fmt.Errorf("no data returned from ls-remote")
	}

	var latestVersionSoFar *semver.Version
	for _, pair := range all {
		// avoid reading slice out of bounds
		if len(pair) < 51 {
			continue
		}
		if pair[46:50] == "tags" {
			vstr := strings.TrimSuffix(string(pair[51:]), "^{}")
			v, err := semver.NewVersion(vstr)
			// skip this tag if it fails to parse as a semantic version
			if err != nil {
				continue
			}
			if latestVersionSoFar == nil || v.GreaterThan(*latestVersionSoFar) {
				latestVersionSoFar = &v
			}
		}
	}
	if latestVersionSoFar == nil {
		return nil, fmt.Errorf("no latest version found")
	}

	return latestVersionSoFar, nil
}
