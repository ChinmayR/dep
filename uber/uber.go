package vcs

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

const (
	// this environment variable is set via go-build in test/production builds.
	// see https://code.uberinternal.com/T397242 for more details
	uberGopkgRedirectEnv = "UBER_GOPKG_FROM_GITOLITE"

	// this is also set via go-build to disable repo autocreation
	uberDisableGitoliteAutocreation = "UBER_NO_GITOLITE_AUTOCREATE"
)

var uberLogger = log.New(os.Stdout, "[UBER]  ", 0)

type rewriteFn func(*GitRepo, []string) (string, error)

type internalRewriter struct {
	log     bool
	name    string
	pattern *regexp.Regexp
	fn      rewriteFn
}

var hostnameRewrites = []internalRewriter{
	{
		name:    "gitolite",
		log:     false,
		pattern: regexp.MustCompile("^https://code.uber.internal/(.*)$"),
		fn:      func(_ *GitRepo, in []string) (string, error) { return gitoliteURI(in[1]), nil },
	},
	{
		name:    "golang.org",
		log:     true,
		pattern: regexp.MustCompile("^https://golang.org/x/(.*)$"),
		fn:      func(_ *GitRepo, in []string) (string, error) { return gitoliteURI("googlesource/" + in[1]), nil },
	},
	{
		name:    "gopkg.in",
		log:     true,
		pattern: regexp.MustCompile(`^https?://gopkg.in/((?P<user>[^./]+)(?P<version>\.v[0-9.]+)?(/(?P<repo>[^./]+)(?P<version>\.v[0-9.]+))?)$`),
		fn:      rewriteGopkgIn,
	},
	// Make sure to keep this last, so the above rules can rewrite to public GitHub
	// and we can rewrite public GitHub to gitolite
	{
		name:    "github.com",
		log:     true,
		pattern: regexp.MustCompile("^https?://github.com/(?P<user>[^/]+)/(?P<repo>[^/]+)$"),
		fn:      getGitoliteMirrorURL,
	},
}

func (s *GitRepo) ignoreRemoteMismatch() bool {
	for _, re := range hostnameRewrites {
		if re.pattern.MatchString(s.remote) {
			return true
		}
	}
	return false
}

func (s *GitRepo) remoteOrGitolite() string {
	for _, rewriter := range hostnameRewrites {
		matches := rewriter.pattern.FindStringSubmatch(s.remote)
		if len(matches) > 0 {
			uri, err := rewriter.fn(s, matches)
			if err != nil {
				uberLogger.Printf("Error rewriting %s: %v", rewriter.name, err)
				continue
			}

			if rewriter.log {
				logRewrite(rewriter.name, s.remote, uri)
			}

			s.remote = uri
		}
	}
	return s.Remote()
}

// isNotOnGitolite returns a value indicating whether the error corresponds to a
// repository not existing on Gitolite.
func (s *GitRepo) isNotOnGitolite(err error) bool {
	return strings.Contains(err.Error(), "FATAL: autocreate denied")
}

// getGitoliteMirrorURL returns a rewritten URL for a GitHub package, using a Gitolite
// mirror instead.  If the repository has not yet been mirrored, it creates the mirror.
func getGitoliteMirrorURL(s *GitRepo, match []string) (string, error) {
	user := match[1]
	repo := match[2]

	// Return with an error if that didn't work for some reason.
	if user == "" || repo == "" {
		return "", fmt.Errorf("could not extract user / repo from GitHub URL: %s", s.Remote())
	}

	return s.ensureGitoliteGithubMirror(user, repo)
}

func (s *GitRepo) ensureGitoliteGithubMirror(user, repo string) (string, error) {
	// Generate the repo path and full URL on Gitolite.
	githubPath := fmt.Sprintf("github/%s/%s", user, repo)
	gitoliteURL := gitoliteURI(githubPath)

	if os.Getenv(uberDisableGitoliteAutocreation) != "" {
		return gitoliteURL, nil
	}

	// Ping Gitolite to see if the mirror exists.
	_, err := s.run("git", "ls-remote", gitoliteURL, "HEAD")

	// If so, nothing more is needed, return the Gitolite mirror URL.
	if err == nil {
		uberLogger.Printf("Gitolite GitHub mirror %s already exists", gitoliteURL)
		return gitoliteURL, nil
	}

	// First, ensure the GitHub repo exists
	if _, err := s.run("git", "ls-remote", s.Remote(), "HEAD"); err != nil {
		uberLogger.Printf("Upstream GitHub repo does not exist: %v", s.Remote())
		return "", err
	}

	// If an error is returned indicating the mirror doesn't exist, create it.
	if !s.isNotOnGitolite(err) {
		return "", err
	}

	uberLogger.Printf("GitHub repo %s does not exist yet on Gitolite, mirroring...", gitoliteURL)

	// Create a mirror.
	_, err = s.run("ssh", "gitolite@code.uber.internal", "create", githubPath)

	// Return with an error if that failed.
	if err != nil {
		return "", err
	}

	// All done.
	uberLogger.Printf("Created Gitolite GitHub mirror %s", gitoliteURL)
	return gitoliteURL, nil
}

func rewriteGopkgIn(s *GitRepo, match []string) (string, error) {
	user, repo := "go-"+match[2], match[2]
	if len(match) > 5 && match[5] != "" {
		user, repo = match[2], match[5]
	}

	// If we're running during a test/production build, assume a .lock file already exists and
	// rewrite to gitolite (which doesn't do any HEAD trickery like gopkg.in does)
	if os.Getenv(uberGopkgRedirectEnv) != "" {
		newRemote := fmt.Sprintf("https://github.com/%s/%s", user, repo)

		return newRemote, nil
	}

	// Otherwise, somebody is running "go-build/glide up" to fetch a new package from gopkg.in
	// Rewrite the remote to https://gopkg.uberinternal.com, which the user will clone from, which
	// **does** do the HEAD rewrite trickery, but also has a dependency on GitHub.

	fullRepo := match[1]

	if _, err := s.ensureGitoliteGithubMirror(user, repo); err != nil {
		uberLogger.Printf("Unable to ensure gitolite mirror exists for underlying GitHub repo for gopkg.in/%s", fullRepo)
		return "", err
	}

	return fmt.Sprintf("https://gopkg.uberinternal.com/%s", fullRepo), nil
}

func gitoliteURI(repo string) string {
	return fmt.Sprintf("gitolite@code.uber.internal:%s", repo)
}

func logRewrite(typ, from, to string) {
	uberLogger.Printf("Rewrite %s %s to %s", typ, from, to)
}
