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
	FILE_PATTERN = "GIT_SSH_COMMAND=ssh -oControlMaster=auto -oControlPath=~/.ssh/%d-%%r@%%h:%%p -oControlPersist=60s"
)

const (
	numThreadsAllowed = 25
)
var threadSema = make(chan ConResource, numThreadsAllowed)

type ConResource int

func init() {
	for i := 1; i <= numThreadsAllowed; i++ {
		go func(localIter int) {
			conRes := ConResource(localIter)
			threadSema <- conRes
			err := conRes.createSocketForGitolite()
			if err != nil {
				UberLogger.Printf("Error initializing thread %d " +
					"(will try again when fetching env var): %s", localIter, err)
			}
		}(i)
	}
}

func (conRes ConResource) createSocketForGitolite() error {
	socketFile := filepath.Join(os.Getenv("HOME"), ".ssh/"+fmt.Sprintf("%d-gitolite@code.uber.internal:2222", conRes))
	if _, err := os.Stat(socketFile); os.IsNotExist(err) {
		// Make a dummy ls-remote call to gitolite to create the socket and cache the connection
		command := exec.Command("git", "ls-remote", "ssh://gitolite@code.uber.internal/devexp/dep", "HEAD")
		command.Env = append([]string{fmt.Sprintf(FILE_PATTERN, conRes)}, os.Environ()...)
		command.Start()

		// Retry every 1 second and check if the socket file exists, if yes then return
		retries := 5
		var timeout <- chan time.Time
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

// GetEnvironmentForCommand gets the environment to append to the command for using the
// cached ssh socket for this call to the specified remote. It first makes sure the socket
// exists in the cache and is warmed up before returning.
func (conRes ConResource) GetEnvironmentForCommand(remote string) []string {
	var retStr string
	if strings.Contains(remote, "code.uber.internal") {
		// make sure the socket file exists first in the cache before making the call
		err := conRes.createSocketForGitolite()
		if err != nil {
			return []string{}
		}
		retStr = fmt.Sprintf(FILE_PATTERN, conRes)
	}
	return []string{retStr}
}

func (conRes ConResource) GetEnvironmentForGitoliteCommand() []string {
	return conRes.GetEnvironmentForCommand("code.uber.internal")
}

func (conRes ConResource) Release() {
	threadSema <- conRes
}

func GetThreadFromPool() ConResource {
	return <-threadSema
}