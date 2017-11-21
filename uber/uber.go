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
)

var uberLogger = log.New(os.Stdout, "[UBER]  ", 0)

type rewriteFn func([]string, ExecutorInterface) (*url.URL, error)

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

func GetGitoliteUrlForRewriter(path, rewriterName string) (*url.URL, error) {
	executor := new(CommandExecutor)
	return useRewriterWithExecutor(path, rewriterName, executor)
}

func useRewriterWithExecutor(path string, rewriterName string, executor ExecutorInterface) (*url.URL, error) {
	rewriter := hostnameRewrites[rewriterName]
	matches := rewriter.pattern.FindStringSubmatch(path)
	if len(matches) > 0 {
		return rewriter.fn(matches, executor)
	}
	return nil, fmt.Errorf("Could not match path: %s to given rewriter: %s", path, rewriterName)

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
	}
	return path
}

func rewriteGitolite(match []string, ex ExecutorInterface) (*url.URL, error) {
	return getGitoliteUrlWithPath(strings.TrimPrefix(GetGitoliteRoot(match[0]), "code.uber.internal/")), nil
}

// rewriteGithub returns a rewritten URL for a GitHub package, using a Gitolite
// mirror instead.  If the repository has not yet been mirrored, it creates the mirror.
func rewriteGithub(match []string, ex ExecutorInterface) (*url.URL, error) {
	user := match[1]
	repo := match[2]

	// Return with an error if that didn't work for some reason.
	if user == "" || repo == "" {
		return nil, fmt.Errorf("could not extract user / repo from GitHub URL: %s", match[0])
	}

	gpath := gitolitePathForGithub(user, repo)
	remote := getGithubRemoteFromUserAndRepo(user, repo)
	return ensureGitoliteMirror(gpath, remote, ex)
}

func ensureGitoliteMirror(gpath, remote string, ex ExecutorInterface) (*url.URL, error) {
	// Generate the full URL on Gitolite.
	gitoliteURL := getGitoliteUrlWithPath(gpath)

	if os.Getenv(UberDisableGitoliteAutocreation) != "" {
		return gitoliteURL, nil
	}

	// Ping Gitolite to see if the mirror exists.
	stdout, stderr, err := ex.ExecCommand("git", "ls-remote", gitoliteURL.String(), "HEAD")
	uberLogger.Print(stdout)

	// If so, nothing more is needed, return the Gitolite mirror URL.
	if err == nil {
		uberLogger.Printf("Gitolite GitHub mirror %s already exists", gitoliteURL)
		return gitoliteURL, nil
	}

	// First, ensure the remote repo exists
	uberLogger.Printf("%s not found on Gitolite, checking %s", gpath, remote)
	rstdout, _, rerr := ex.ExecCommand("git", "ls-remote", remote, "HEAD")
	uberLogger.Print(rstdout)
	if rerr != nil {
		uberLogger.Printf("Upstream repo does not exist: %v", remote)
		return nil, rerr
	}

	// If an error is returned indicating the mirror doesn't exist, create it.
	if !strings.Contains(stderr, "FATAL: autocreate denied") {
		// the error was something other than the repo not existing in Gitolite
		return nil, err
	}

	uberLogger.Printf("Remote repo %s does not exist yet on Gitolite, mirroring...", gitoliteURL)

	// Create a mirror.
	stdout, stderr, err = ex.ExecCommand("ssh", "gitolite@code.uber.internal", "create", gpath)
	uberLogger.Print(stdout)

	// Return with an error if that failed.
	if err != nil {
		uberLogger.Print(stderr)
		uberLogger.Printf("Error creating repo %s on Gitolite: %s", gpath, err.Error())
		return nil, err
	}

	// All done.
	uberLogger.Printf("Created Gitolite GitHub mirror %s", gitoliteURL)
	return gitoliteURL, nil
}

func rewriteGopkgIn(match []string, ex ExecutorInterface) (*url.URL, error) {
	user, repo := "go-"+match[2], match[2]
	if len(match) > 5 && match[5] != "" {
		user, repo = match[2], match[5]
	}
	gpath := gitolitePathForGithub(user, repo)

	// If we're running during a test/production build, assume a .lock file already exists and
	// rewrite to gitolite (which doesn't do any HEAD trickery like gopkg.in does)
	if os.Getenv(UberGopkgRedirectEnv) != "" {
		return getGitoliteUrlWithPath(gpath), nil
	}

	// Otherwise, somebody is running "go-build/glide up" to fetch a new package from gopkg.in
	// Rewrite the remote to https://gopkg.uberinternal.com, which the user will clone from, which
	// **does** do the HEAD rewrite trickery, but also has a dependency on GitHub.

	fullRepo := match[1]
	remote := getGithubRemoteFromUserAndRepo(user, repo)
	if _, err := ensureGitoliteMirror(gpath, remote, ex); err != nil {
		uberLogger.Printf("Unable to ensure gitolite mirror exists for underlying GitHub repo for gopkg.in/%s", fullRepo)
		return nil, err
	}

	u := new(url.URL)
	u.Host = "gopkg.uberinternal.com"
	u.Scheme = "https"
	u.Path = "/" + fullRepo
	return u, nil
}

func rewriteGolang(in []string, ex ExecutorInterface) (*url.URL, error) {
	repo := in[1]
	gpath := gitolitePathForGolang(repo)
	remote := getGolangRemoteFromRepo(repo)
	return ensureGitoliteMirror(gpath, remote, ex)
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
