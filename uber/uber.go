package uber

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
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
)

const UBER_PREFIX = "[UBER]  "

var UberLogger = log.New(os.Stdout, UBER_PREFIX, 0)

type rewriteFn func([]string, ExecutorInterface) (*url.URL, string, string, *url.URL, error)

type internalRewriter struct {
	log     bool
	pattern *regexp.Regexp
	fn      rewriteFn
}

type ExecutorInterface interface {
	ExecCommand(name string, arg ...string) (string, string, error)
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
	UberLogger.Printf("Found unambigious path, assuming .git for %s\n", path)
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

	// Ping Gitolite to see if the mirror exists.
	stdout, stderr, err := ex.ExecCommand("git", "ls-remote", gitoliteURL.String(), "HEAD")

	// If so, nothing more is needed, return the Gitolite mirror URL.
	if err == nil {
		return nil
	}

	UberLogger.Printf("project not found at URL: %s", gitoliteURL.String())

	// First, ensure the remote repo exists
	UberLogger.Printf("%s not found on Gitolite, checking %s", gpath, remote)
	rstdout, _, rerr := ex.ExecCommand("git", "ls-remote", remote, "HEAD")
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
	stdout, stderr, err = ex.ExecCommand("ssh", "gitolite@code.uber.internal", "create", mirrorGpath)
	UberLogger.Print(stdout)

	// Return with an error if that failed.
	if err != nil {
		UberLogger.Print(stderr)
		UberLogger.Printf("Error creating repo %s on Gitolite: %s", gpath, err.Error())
		return err
	}

	// All done.
	UberLogger.Printf("Created Gitolite GitHub mirror %s", gitoliteURL)
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
	return fmt.Sprintf("git@github.com:%s/%s", user, repo)
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

func (c *CommandExecutor) ExecCommand(name string, arg ...string) (string, string, error) {
	command := exec.Command(name, arg...)
	command.Env = os.Environ()
	var stdoutbytes, stderrbytes bytes.Buffer
	command.Stdout = &stdoutbytes
	command.Stderr = &stderrbytes
	err := command.Run()
	return stdoutbytes.String(), stderrbytes.String(), err
}
