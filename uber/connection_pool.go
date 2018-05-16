package uber

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	FILE_PATTERN = "GIT_SSH_COMMAND=ssh -oControlMaster=auto -oControlPath=%s/%d-%%r@%%h:%%p -oControlPersist=60s"
)

const (
	numThreadsAllowed = 20
)

var threadSema = make(chan ConResource, numThreadsAllowed)

type ConResource struct {
	threadNum    int
	cacheEnabled bool
}

func init() {
	for i := 1; i <= numThreadsAllowed; i++ {
		go func(localIter int) {
			conRes := ConResource{threadNum: localIter, cacheEnabled: false}
			// attempting to cache the ssh connection is best effort and so we continue in case of failure
			conRes.deleteSocketForGitolite()
			err := conRes.createSocketForGitolite()
			conRes.cacheEnabled = err == nil
			threadSema <- conRes
		}(i)
	}
}

func GetCacheDir() string {
	return filepath.Join(os.Getenv("HOME"), ".dep-cache", "pkg")
}

func (conRes ConResource) createSocketForGitolite() error {
	socketFile := filepath.Join(GetCacheDir(), fmt.Sprintf("%d-gitolite@code.uber.internal:2222", conRes.threadNum))
	if _, err := os.Stat(socketFile); os.IsNotExist(err) {
		// Make a dummy ls-remote call to gitolite to create the socket and cache the connection
		command := exec.Command("git", "ls-remote", "ssh://gitolite@code.uber.internal/devexp/dep", "HEAD")
		command.Env = append([]string{fmt.Sprintf(FILE_PATTERN, GetCacheDir(), conRes.threadNum)}, os.Environ()...)
		command.Start()

		// Retry every 1 second and check if the socket file exists, if yes then return
		retries := 5
		var timeout <-chan time.Time
		for retries > 0 {
			timeout = time.After(1 * time.Second)
			select {
			case <-timeout:
				if _, err2 := os.Stat(socketFile); err2 == nil {
					return nil
				}
			}
			retries--
		}
		return errors.Wrapf(err, "unable to create socket file %s", socketFile)
	}
	return nil
}

func (conRes ConResource) deleteSocketForGitolite() error {
	socketFile := filepath.Join(GetCacheDir(), fmt.Sprintf("%d-gitolite@code.uber.internal:2222", conRes.threadNum))
	if _, err := os.Stat(socketFile); err == nil {
		err := os.Remove(socketFile)
		if err != nil {
			return errors.Wrapf(err, "unable to delete socket file %s", socketFile)
		}
	}
	return nil
}

// GetEnvironmentForCommand gets the environment to append to the command for using the
// cached ssh socket for this call to the specified remote. It first makes sure the socket
// exists in the cache and is warmed up before returning.
func (conRes ConResource) GetEnvironmentForCommand(remote string) []string {
	var retStr string
	if conRes.cacheEnabled && strings.Contains(remote, "code.uber.internal") {
		// make sure the socket file exists first in the cache before making the call
		err := conRes.createSocketForGitolite()
		if err != nil {
			conRes.cacheEnabled = false
			return []string{}
		}
		retStr = fmt.Sprintf(FILE_PATTERN, GetCacheDir(), conRes.threadNum)
	}
	return []string{retStr}
}

func (conRes ConResource) GetEnvironmentForGitoliteCommand() []string {
	return conRes.GetEnvironmentForCommand("code.uber.internal")
}

func (conRes ConResource) Release() {
	UberLogger.Printf("Length of connection pool: %v\n", len(threadSema))
	threadSema <- conRes
}

func GetThreadFromPool() ConResource {
	UberLogger.Printf("Length of connection pool: %v\n", len(threadSema))
	return <-threadSema
}
