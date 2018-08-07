package uber

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

const (
	// this environment variable is set via go-build in test/production builds.
	// see https://code.uberinternal.com/T397242 for more details
	UberGopkgRedirectEnv = "UBER_GOPKG_FROM_GITOLITE"

	// this is also set via go-build to disable repo autocreation
	UberDisableGitoliteAutocreation = "UBER_NO_GITOLITE_AUTOCREATE"

	// this can be set by developers that want to turn off the slower uber specific
	// checking + mirroring logic
	TurnOffUberDeduceLogicEnv = "TURN_OFF_UBER_DEP_DEDUCE_LOGIC"

	// this flag forces dep to list all versions on a remote package repository
	// it can have a performance impact because dep will try to use the packages
	// from all possible refs on a remote repository
	UseNonDefaultVersionBranches = "USE_NON_DEFAULT_VERSION_BRANCHES"

	// this flag is used as a feature flag to control UBER specific features to
	// avoid modifying the existing integration tests from upstream
	RunningIntegrationTests = "RUNNING_INTEGRATION_TESTS"

	// this flag is used to turn off metrics reporting when dep is run via
	// automated scripts to onboard customers. We do not want to inflate adoption
	// or usage in these cases.
	TurnOffMetricsReporting = "TURN_OFF_METRICS_REPORTING"
)

const DEP_VERSION = "v1.1.0-UBER"
const LATEST_CACHE_ALLOWED_VERSION = "v0.11.0-UBER"

type rewriteFn func([]string, ExecutorInterface) (*url.URL, string, string, *url.URL, error)

type internalRewriter struct {
	log     bool
	pattern *regexp.Regexp
	fn      rewriteFn
}

type ExecutorInterface interface {
	ExecCommand(name string, cmdTimeout time.Duration, runInBackground bool, environment []string, arg ...string) (string, string, error)
}

type CommandExecutor struct {
}

var hostnameRewrites = map[string]internalRewriter{
	"github.com": {
		log:     true,
		pattern: regexp.MustCompile("^github.com/(?P<user>[^/]+)/(?P<repo>[^/]+)"),
		fn:      rewriteGithub,
	},
	"gopkg.in": {
		log:     true,
		pattern: regexp.MustCompile(`^gopkg.in/((?P<user>[^./]+)(?P<version>\.v[0-9.]+)?(/(?P<repo>[^./]+)(?P<version>\.v[0-9.]+))?)`),
		fn:      rewriteGopkgIn,
	},
	"golang.org": {
		log:     true,
		pattern: regexp.MustCompile("^golang.org/x/([^/]+)"),
		fn:      rewriteGolang,
	},
	"code.uber.internal": {
		log:     true,
		pattern: regexp.MustCompile("^code.uber.internal/(.+)$"),
		fn:      rewriteGitolite,
	},
}

func GetGitoliteUrlForRewriter(path, rewriterName string) (*url.URL, string, string, *url.URL, error) {
	executor := new(CommandExecutor)
	return useRewriterWithExecutor(path, rewriterName, executor)
}

func useRewriterWithExecutor(path string, rewriterName string, executor ExecutorInterface) (*url.URL, string, string, *url.URL, error) {
	rewriter := hostnameRewrites[rewriterName]
	matches := rewriter.pattern.FindStringSubmatch(path)
	if len(matches) > 0 {
		return rewriter.fn(matches, executor)
	}
	return nil, "", "", nil, fmt.Errorf("Could not match path: %s to given rewriter: %s", path, rewriterName)

}

// getGitoliteUrlWithPath returns a URL pointing to the repo specified by path on gitolite.
// (ex "github/uber/tchannel-go", "googlesource/net", etc)
// path should not contain a leading "/"
func getGitoliteUrlWithPath(path string) *url.URL {
	u := new(url.URL)
	u.User = url.User("gitolite")
	u.Host = "code.uber.internal"
	u.Scheme = "ssh"
	// The leading "/" only needs to be present for equivalency in unit tests. Urls are correctly interpreted
	// whether the path member has the leading "/" or not.
	u.Path = "/" + path
	return u
}

func GetGitoliteRoot(path string) string {
	if strings.Contains(path, ".git") {
		return strings.SplitAfter(path, ".git")[0]
	} else if len(strings.Split(path, "/")) > 3 {
		splitPath := strings.Split(path, "/")
		assumedPath := splitPath[0] + "/" + splitPath[1] + "/" + splitPath[2]
		UberLogger.Printf("Found ambigious path, taking %s for %s\n", assumedPath, path)
		return assumedPath
	}
	return path
}

func rewriteGitolite(match []string, ex ExecutorInterface) (*url.URL, string, string, *url.URL, error) {
	gitoliteURL := getGitoliteUrlWithPath(strings.TrimPrefix(GetGitoliteRoot(match[0]), "code.uber.internal/"))
	return gitoliteURL, "", "", gitoliteURL, nil
}

// rewriteGithub returns a rewritten URL for a GitHub package, using a Gitolite
// mirror instead.  If the repository has not yet been mirrored, it creates the mirror.
func rewriteGithub(match []string, ex ExecutorInterface) (*url.URL, string, string, *url.URL, error) {
	user := match[1]
	repo := match[2]

	// Return with an error if that didn't work for some reason.
	if user == "" || repo == "" {
		return nil, "", "", nil, fmt.Errorf("could not extract user / repo from GitHub URL: %s", match[0])
	}

	gpath := gitolitePathForGithub(user, repo)
	remote := getGithubRemoteFromUserAndRepo(user, repo)
	gitoliteURL := getGitoliteUrlWithPath(gpath)
	return gitoliteURL, gpath, remote, gitoliteURL, nil
}

func CheckAndMirrorRepo(ex ExecutorInterface, gpath, remote string, gitoliteURL *url.URL) error {
	if os.Getenv(UberDisableGitoliteAutocreation) != "" {
		return nil
	}

	conRes := GetThreadFromPool()
	defer conRes.Release()

	// Ping Gitolite to see if the mirror exists.
	_, stderr, err := ex.ExecCommand("git", time.Duration(1*time.Minute), false, conRes.GetEnvironmentForGitoliteCommand(),
		"ls-remote", gitoliteURL.String(), "HEAD")

	// If so, nothing more is needed, return the Gitolite mirror URL.
	if err == nil {
		return nil
	}

	UberLogger.Printf("project not found at URL: %s", gitoliteURL.String())

	// First, ensure the remote repo exists
	UberLogger.Printf("%s not found on Gitolite, checking %s", gpath, remote)
	rstdout, _, rerr := ex.ExecCommand("git", time.Duration(1*time.Minute), false, conRes.GetEnvironmentForGitoliteCommand(),
		"ls-remote", remote, "HEAD")
	UberLogger.Print(rstdout)
	if rerr != nil {
		UberLogger.Printf("Upstream repo does not exist: %v", remote)
		UberLogger.Print(rerr)
		panic("Failed to mirror repo, this will cause problems later in the flow")
		return rerr
	}

	// If an error is returned indicating the mirror doesn't exist, create it.
	if !strings.Contains(stderr, "FATAL: autocreate denied") {
		// the error was something other than the repo not existing in Gitolite
		return err
	}

	UberLogger.Printf("Remote repo %s does not exist yet on Gitolite, mirroring...", gitoliteURL)

	// Create a mirror.
	mirrorGpath := strings.Replace(gpath, ".git", "", -1)
	_, _, err = ex.ExecCommand("ssh", time.Duration(2*time.Minute), true, conRes.GetEnvironmentForGitoliteCommand(),
		"gitolite@code.uber.internal", "create", mirrorGpath)

	// Return with an error if that failed.
	if err != nil {
		UberLogger.Printf("Error creating repo %s on Gitolite: %s", gpath, err.Error())
		return err
	}

	// All done.
	return nil
}

func rewriteGopkgIn(match []string, ex ExecutorInterface) (*url.URL, string, string, *url.URL, error) {
	user, repo := "go-"+match[2], match[2]
	if len(match) > 5 && match[5] != "" {
		user, repo = match[2], match[5]
	}
	gpath := gitolitePathForGithub(user, repo)

	// If we're running during a test/production build, assume a .lock file already exists and
	// rewrite to gitolite (which doesn't do any HEAD trickery like gopkg.in does)
	if os.Getenv(UberGopkgRedirectEnv) != "" {
		gitoliteURL := getGitoliteUrlWithPath(gpath)
		return gitoliteURL, "", "", gitoliteURL, nil
	}

	// Otherwise, somebody is running "go-build/glide up" to fetch a new package from gopkg.in
	// Rewrite the remote to https://gopkg.uberinternal.com, which the user will clone from, which
	// **does** do the HEAD rewrite trickery, but also has a dependency on GitHub.

	fullRepo := match[1]
	remote := getGithubRemoteFromUserAndRepo(user, repo)
	gitoliteURL := getGitoliteUrlWithPath(gpath)

	u := new(url.URL)
	u.Host = "gopkg.uberinternal.com"
	u.Scheme = "https"
	u.Path = "/" + fullRepo
	return u, gpath, remote, gitoliteURL, nil
}

func rewriteGolang(in []string, ex ExecutorInterface) (*url.URL, string, string, *url.URL, error) {
	repo := in[1]
	gpath := gitolitePathForGolang(repo)
	remote := getGolangRemoteFromRepo(repo)
	gitoliteURL := getGitoliteUrlWithPath(gpath)
	return gitoliteURL, gpath, remote, gitoliteURL, nil
}

func getGithubRemoteFromUserAndRepo(user, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s", user, repo)
}

func gitolitePathForGithub(user, repo string) string {
	return fmt.Sprintf("github/%s/%s", user, repo)
}

func getGolangRemoteFromRepo(repo string) string {
	return fmt.Sprintf("https://go.googlesource.com/%s", repo)
}

func gitolitePathForGolang(repo string) string {
	return fmt.Sprintf("googlesource/%s", repo)
}

// ExecCommand creates and executes the command defined by the name and args fields
// The command can be specified to run in background via the runInBackground flag
// which will only return an error if there is a problem starting the command.
// If run in background is not specified then the command can be given a timeout
// duration which will cause the command to time out and return an error, unless
// the command completes successfully first.
func (c *CommandExecutor) ExecCommand(name string, cmdTimeout time.Duration, runInBackground bool, environment []string, arg ...string) (string, string, error) {
	command := exec.Command(name, arg...)
	command.Env = append([]string{"GIT_ASKPASS=", "GIT_TERMINAL_PROMPT=0"}, append(environment, os.Environ()...)...)

	DebugLogger.Printf("executing command: %v %v, timeout: %s, environment: %v, background: %t", name, arg, cmdTimeout, environment, runInBackground)

	// Start a timer
	timeout := time.After(cmdTimeout)
	// Force subprocesses into their own process group, rather than being in the
	// same process group as the dep process. Because Ctrl-C sent from a
	// terminal will send the signal to the entire currently running process
	// group, this allows us to directly manage the issuance of signals to
	// subprocesses.
	command.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	var stdoutbytes, stderrbytes bytes.Buffer
	command.Stdout = &stdoutbytes
	command.Stderr = &stderrbytes
	if err := command.Start(); err != nil {
		return "", "", err
	}

	if !runInBackground {
		done := make(chan error)
		go func() { done <- command.Wait() }()

		select {
		case <-timeout:
			command.Process.Kill()
			DebugLogger.Printf("timed out command: %v %v, timeout: %s, environment: %v, background: %t, err: %v, out: %v", name, arg, cmdTimeout, environment, runInBackground, string(stderrbytes.Bytes()), string(stdoutbytes.Bytes()))
			return "", "", errors.New("Command timed out")
		case err := <-done:
			DebugLogger.Printf("successful command: %v %v, timeout: %s, environment: %v, background: %t, err: %v, out: %v", name, arg, cmdTimeout, environment, runInBackground, err, string(stdoutbytes.Bytes()))
			return stdoutbytes.String(), stderrbytes.String(), err
		}
	}

	DebugLogger.Printf("successful command: %v %v, timeout: %s, environment: %v, background: %t", name, arg, cmdTimeout, environment, runInBackground)
	return "", "", nil
}
