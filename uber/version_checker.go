package uber

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
)

const (
	REMOTE_URL            = "ssh://gitolite@code.uber.internal/devexp/dep"
	CACHE_CLEAR_FILE_NAME = "cache_clear"
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

// DoesCacheNeedToBeCleared reads from the CACHE_CLEAR_FILE_NAME file from pkg/dep cache
// and if it is older than the minVersionAllowed then the cache is backwards incompatible
// and needs to be cleared
func DoesCacheNeedToBeCleared(minVersionAllowed string) (bool, string, error) {
	cacheDir := filepath.Join(os.Getenv("HOME"), ".dep-cache", "pkg", "dep")
	clearCachePath := filepath.Join(cacheDir, CACHE_CLEAR_FILE_NAME)
	clearCacheFileContent, err := ioutil.ReadFile(clearCachePath)
	if err != nil {
		return true, "", errors.Wrap(err, "failed to read clear cache file")
	}
	clearCacheFileContentString := strings.Split(string(clearCacheFileContent), "\n")[0]
	cacheClearedVersion, err := semver.NewVersion(clearCacheFileContentString)
	if err != nil {
		return true, "", errors.Wrap(err, "failed to parse clear cache file contents")
	}
	minVersionAllowedSemver, err := semver.NewVersion(minVersionAllowed)
	if err != nil {
		return true, cacheClearedVersion.String(), errors.Wrap(err, "failed to parse input min version allowed")
	}
	return cacheClearedVersion.Compare(minVersionAllowedSemver) < 0, cacheClearedVersion.String(), nil
}

func WriteCacheClearedVersion(version string, cacheDir string) error {
	// Create the default cachedir if it does not exist.
	if err := os.MkdirAll(cacheDir, 0777); err != nil {
		return errors.Wrap(err, "failed to create default cache directory")
	}

	clearCachePath := filepath.Join(cacheDir, CACHE_CLEAR_FILE_NAME)

	clearCacheFile, err := os.OpenFile(clearCachePath, os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		errors.Wrap(err, "failed to open new clear cache file")
	}
	defer clearCacheFile.Close()
	clearCacheFile.WriteString(version)

	return nil
}
