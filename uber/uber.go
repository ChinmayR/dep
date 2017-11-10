package uber

import (
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

type rewriteFn func(string, []string) (*url.URL, error)

type internalRewriter struct {
	log     bool
	pattern *regexp.Regexp
	fn      rewriteFn
}

var hostnameRewrites = map[string]internalRewriter{
	"github.com": {
		log:     true,
		pattern: regexp.MustCompile("^github.com/(?P<user>[^/]+)/(?P<repo>[^/]+)"),
		fn:      getGitoliteMirrorURL,
	},
	"gopkg.in": {
		log:     true,
		pattern: regexp.MustCompile(`^gopkg.in/((?P<user>[^./]+)(?P<version>\.v[0-9.]+)?(/(?P<repo>[^./]+)(?P<version>\.v[0-9.]+))?)`),
		fn:      rewriteGopkgIn,
	},
}

func GetGitoliteUrlForRewriter(path, rewriterName string) (*url.URL, error) {
	rewriter := hostnameRewrites[rewriterName]
	matches := rewriter.pattern.FindStringSubmatch(path)
	if len(matches) > 0 {
		return rewriter.fn(path, matches)
	}
	return nil, fmt.Errorf("Could not match path: %s to given rewriter: %s", path, rewriterName)
}

func GetGitoliteUrlWithPath(path string) *url.URL {
	u := new(url.URL)
	u.User = url.User("gitolite")
	u.Host = "code.uber.internal"
	u.Scheme = "ssh"
	u.Path = strings.TrimPrefix(GetGitoliteRoot(path), u.Host)
	return u
}

func GetGitoliteRoot(path string) string {
	if strings.Contains(path, ".git") {
		return strings.SplitAfter(path, ".git")[0]
	}
	return path
}

// isNotOnGitolite returns a value indicating whether the error corresponds to a
// repository not existing on Gitolite.
func isNotOnGitolite(err error) bool {
	return strings.Contains(err.Error(), "FATAL: autocreate denied")
}

// getGitoliteMirrorURL returns a rewritten URL for a GitHub package, using a Gitolite
// mirror instead.  If the repository has not yet been mirrored, it creates the mirror.
func getGitoliteMirrorURL(path string, match []string) (*url.URL, error) {
	user := match[1]
	repo := match[2]

	// Return with an error if that didn't work for some reason.
	if user == "" || repo == "" {
		return nil, fmt.Errorf("could not extract user / repo from GitHub URL: %s", path)
	}

	return ensureGitoliteGithubMirror(user, repo, path)
}

func ensureGitoliteGithubMirror(user, repo, path string) (*url.URL, error) {
	// Generate the repo path and full URL on Gitolite.
	githubPath := fmt.Sprintf("github/%s/%s", user, repo)
	gitoliteURL := GetGitoliteUrlWithPath("/" + githubPath)

	if os.Getenv(UberDisableGitoliteAutocreation) != "" {
		return gitoliteURL, nil
	}

	// Ping Gitolite to see if the mirror exists.
	gitoliteUri := fmt.Sprintf("%s@%s:%s", gitoliteURL.User.Username(), gitoliteURL.Hostname(), gitoliteURL.Path)
	err := execAndLogCommand("git", "ls-remote", gitoliteUri, "HEAD")

	// If so, nothing more is needed, return the Gitolite mirror URL.
	if err == nil {
		uberLogger.Printf("Gitolite GitHub mirror %s already exists", gitoliteURL)
		return gitoliteURL, nil
	}

	// First, ensure the GitHub repo exists
	githubUrlString := getGithubUrlFromUserAndRepo(user, repo)
	if execErr := execAndLogCommand("git", "ls-remote", githubUrlString, "HEAD"); execErr != nil {
		uberLogger.Printf("Upstream GitHub repo does not exist: %v", githubUrlString)
		return nil, execErr
	}

	// If an error is returned indicating the mirror doesn't exist, create it.
	if !isNotOnGitolite(err) {
		return nil, err
	}

	uberLogger.Printf("GitHub repo %s does not exist yet on Gitolite, mirroring...", gitoliteURL)

	// Create a mirror.
	err = execAndLogCommand("ssh", "gitolite@code.uber.internal", "create", githubPath)

	// Return with an error if that failed.
	if err != nil {
		return nil, err
	}

	// All done.
	uberLogger.Printf("Created Gitolite GitHub mirror %s", gitoliteURL)
	return gitoliteURL, nil
}

func rewriteGopkgIn(path string, match []string) (*url.URL, error) {
	user, repo := "go-"+match[2], match[2]
	if len(match) > 5 && match[5] != "" {
		user, repo = match[2], match[5]
	}

	// If we're running during a test/production build, assume a .lock file already exists and
	// rewrite to gitolite (which doesn't do any HEAD trickery like gopkg.in does)
	if os.Getenv(UberGopkgRedirectEnv) != "" {
		newRemote := fmt.Sprintf("%s/%s", user, repo)
		u := new(url.URL)
		u.Host = "github.com"
		u.Scheme = "https"
		u.Path = newRemote

		return u, nil
	}

	// Otherwise, somebody is running "go-build/glide up" to fetch a new package from gopkg.in
	// Rewrite the remote to https://gopkg.uberinternal.com, which the user will clone from, which
	// **does** do the HEAD rewrite trickery, but also has a dependency on GitHub.

	fullRepo := match[1]

	if _, err := ensureGitoliteGithubMirror(user, repo, path); err != nil {
		uberLogger.Printf("Unable to ensure gitolite mirror exists for underlying GitHub repo for gopkg.in/%s", fullRepo)
		return nil, err
	}

	u := new(url.URL)
	u.Host = "gopkg.uberinternal.com"
	u.Scheme = "https"
	u.Path = fullRepo
	return u, nil
}

func logRewrite(typ string, from string, to *url.URL) {
	uberLogger.Printf("Rewrite %s %s to %s", typ, from, to)
}

func getGithubUrlFromUserAndRepo(user, repo string) string {
	return fmt.Sprintf("git@github.com:%s/%s", user, repo)
}

func execAndLogCommand(name string, arg ...string) error {
	command := exec.Command(name, arg...)
	command.Env = os.Environ()
	stdout, err := command.Output()
	uberLogger.Printf("%s", stdout)
	return err
}
