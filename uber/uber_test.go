// testing the uber-specific hacks since 2016
package uber

import (
	"testing"
	"time"

	"fmt"
	"net/url"

	"github.com/golang/dep/uber/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type repoTestCase struct {
	given, expUrl, expGpath, expRemote, expGitoliteUrl string
	redirect                                           bool
	autocreate                                         bool
}

func TestUber_IsGopkg(t *testing.T) {
	cases := []repoTestCase{
		{
			given:          "gopkg.in/validator.v2",
			expUrl:         "https://gopkg.uberinternal.com/validator.v2",
			expGpath:       "github/go-validator/validator",
			expRemote:      "https://github.com/go-validator/validator",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			autocreate:     false,
		},
		{
			given:          "gopkg.in/validator.v2",
			expUrl:         "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			redirect:       true,
		},
		{
			given:          "gopkg.in/go-validator/validator.v2",
			expUrl:         "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			redirect:       true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if c.redirect {
				defer SetAndUnsetEnvVar(UberGopkgRedirectEnv, "yes")()
			}

			if !c.autocreate {
				defer SetAndUnsetEnvVar(UberDisableGitoliteAutocreation, "yes")()
			}
			gotUrl, gotGpath, gotRemote, gotGitoliteURL, err := GetGitoliteUrlForRewriter(c.given, "gopkg.in")

			assert.Nil(t, err)
			assert.Equal(t, c.expUrl, gotUrl.String())
			assert.Equal(t, c.expGpath, gotGpath)
			assert.Equal(t, c.expRemote, gotRemote)
			assert.Equal(t, c.expGitoliteUrl, gotGitoliteURL.String())
		}(c)
	}
}

func TestUber_IsGitoliteForGitolite(t *testing.T) {
	cases := []repoTestCase{
		{
			given:          "code.uber.internal/go-common.git",
			expUrl:         "ssh://gitolite@code.uber.internal/go-common.git",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/go-common.git",
		},
		{
			given:          "code.uber.internal/go-common.git/blah",
			expUrl:         "ssh://gitolite@code.uber.internal/go-common.git",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/go-common.git",
		},
		{
			given:          "code.uber.internal/rt/filter.git",
			expUrl:         "ssh://gitolite@code.uber.internal/rt/filter.git",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/rt/filter.git",
		},
		{
			given:          "code.uber.internal/rt/filter",
			expUrl:         "ssh://gitolite@code.uber.internal/rt/filter",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/rt/filter",
		},
		{
			given:          "code.uber.internal/rt/filter/.gen/go/filter",
			expUrl:         "ssh://gitolite@code.uber.internal/rt/filter",
			expGpath:       "",
			expRemote:      "",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/rt/filter",
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			gotUrl, gotGpath, gotRemote, gotGitoliteURL, err := GetGitoliteUrlForRewriter(c.given, "code.uber.internal")

			assert.Nil(t, err)
			assert.Equal(t, c.expUrl, gotUrl.String())
			assert.Equal(t, c.expGpath, gotGpath)
			assert.Equal(t, c.expRemote, gotRemote)
			assert.Equal(t, c.expGitoliteUrl, gotGitoliteURL.String())
		}(c)
	}
}

func TestUber_IsGitoliteForGolang(t *testing.T) {
	cases := []repoTestCase{
		{
			given:          "golang.org/x/net",
			expUrl:         "ssh://gitolite@code.uber.internal/googlesource/net",
			expGpath:       "googlesource/net",
			expRemote:      "https://go.googlesource.com/net",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/googlesource/net",
		},
		{
			given:          "golang.org/x/net/ipv4",
			expUrl:         "ssh://gitolite@code.uber.internal/googlesource/net",
			expGpath:       "googlesource/net",
			expRemote:      "https://go.googlesource.com/net",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/googlesource/net",
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			gotUrl, gotGpath, gotRemote, gotGitoliteURL, err := GetGitoliteUrlForRewriter(c.given, "golang.org")

			assert.Nil(t, err)
			assert.Equal(t, c.expUrl, gotUrl.String())
			assert.Equal(t, c.expGpath, gotGpath)
			assert.Equal(t, c.expRemote, gotRemote)
			assert.Equal(t, c.expGitoliteUrl, gotGitoliteURL.String())
		}(c)
	}
}

func TestUber_IsGitoliteForGithub(t *testing.T) {
	cases := []repoTestCase{
		{
			given:          "github.com/Masterminds/glide",
			expUrl:         "ssh://gitolite@code.uber.internal/github/Masterminds/glide",
			expGpath:       "github/Masterminds/glide",
			expRemote:      "https://github.com/Masterminds/glide",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/Masterminds/glide",
			autocreate:     true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if !c.autocreate {
				defer SetAndUnsetEnvVar(UberDisableGitoliteAutocreation, "yes")()
			}
			gotUrl, gotGpath, gotRemote, gotGitoliteURL, err := GetGitoliteUrlForRewriter(c.given, "github.com")

			assert.Nil(t, err)
			assert.Equal(t, c.expUrl, gotUrl.String(), "Expected URL: ")
			assert.Equal(t, c.expGpath, gotGpath, "Expected Gpath: ")
			assert.Equal(t, c.expRemote, gotRemote, "Expected remote: ")
			assert.Equal(t, c.expGitoliteUrl, gotGitoliteURL.String(), "Expected gitoliteURL: ")
		}(c)
	}
}

func TestUber_IsGitoliteForHonnefco(t *testing.T) {
	cases := []repoTestCase{
		{
			given:          "honnef.co/go/tools/staticcheck",
			expUrl:         "ssh://gitolite@code.uber.internal/github/dominikh/go-tools",
			expGpath:       "github/dominikh/go-tools",
			expRemote:      "https://github.com/dominikh/go-tools",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/dominikh/go-tools",
			autocreate:     true,
		},
		{
			given:          "honnef.co/go/irc",
			expUrl:         "ssh://gitolite@code.uber.internal/github/dominikh/go-irc",
			expGpath:       "github/dominikh/go-irc",
			expRemote:      "https://github.com/dominikh/go-irc",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/dominikh/go-irc",
			autocreate:     true,
		},
		{
			given:          "honnef.co/go/js/dom/blah_pkg",
			expUrl:         "ssh://gitolite@code.uber.internal/github/dominikh/go-js-dom",
			expGpath:       "github/dominikh/go-js-dom",
			expRemote:      "https://github.com/dominikh/go-js-dom",
			expGitoliteUrl: "ssh://gitolite@code.uber.internal/github/dominikh/go-js-dom",
			autocreate:     true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if !c.autocreate {
				defer SetAndUnsetEnvVar(UberDisableGitoliteAutocreation, "yes")()
			}
			gotUrl, gotGpath, gotRemote, gotGitoliteURL, err := GetGitoliteUrlForRewriter(c.given, "honnef.co")

			assert.Nil(t, err)
			assert.Equal(t, c.expUrl, gotUrl.String(), "Expected URL: ")
			assert.Equal(t, c.expGpath, gotGpath, "Expected Gpath: ")
			assert.Equal(t, c.expRemote, gotRemote, "Expected remote: ")
			assert.Equal(t, c.expGitoliteUrl, gotGitoliteURL.String(), "Expected gitoliteURL: ")
		}(c)
	}
}

func TestUber_MirrorsToGitolite(t *testing.T) {

	type mirrorTestCase struct {
		importPath   string
		remoteUrl    string
		gpath        string // the repo path on gitolite: code.uber.internal/<gpath> ex: github/user/repo or googlesource/net
		mirrorGpath  string // sanitized gpath capable of mirroring the repo on gitolite
		rewritername string
		expected     string
		remoteExists bool
	}

	cases := []mirrorTestCase{
		{
			importPath:   "github.com/test/repo",
			remoteUrl:    "https://github.com/test/repo",
			gpath:        "github/test/repo",
			mirrorGpath:  "github/test/repo",
			rewritername: "github.com",
			expected:     "ssh://gitolite@code.uber.internal/github/test/repo",
			remoteExists: true,
		},
		{
			importPath:   "github.com/test/repo.git",
			remoteUrl:    "https://github.com/test/repo.git",
			gpath:        "github/test/repo.git",
			mirrorGpath:  "github/test/repo",
			rewritername: "github.com",
			expected:     "ssh://gitolite@code.uber.internal/github/test/repo.git",
			remoteExists: true,
		},
		{
			importPath:   "gopkg.in/repo.v0",
			remoteUrl:    "https://github.com/go-repo/repo",
			gpath:        "github/go-repo/repo",
			mirrorGpath:  "github/go-repo/repo",
			rewritername: "gopkg.in",
			expected:     "ssh://gitolite@code.uber.internal/github/go-repo/repo",
			remoteExists: true,
		},
		{
			importPath:   "golang.org/x/repo",
			remoteUrl:    "https://go.googlesource.com/repo",
			gpath:        "googlesource/repo",
			mirrorGpath:  "googlesource/repo",
			rewritername: "golang.org",
			expected:     "ssh://gitolite@code.uber.internal/googlesource/repo",
			remoteExists: true,
		},
		{
			importPath:   "golang.org/x/repo",
			gpath:        "googlesource/repo",
			mirrorGpath:  "googlesource/repo",
			rewritername: "golang.org",
			expected:     "ssh://gitolite@code.uber.internal/googlesource/repo",
			remoteExists: false,
		},
	}

	for _, c := range cases {
		func(c mirrorTestCase) {
			ex := &mocks.ExecutorInterface{}
			//does not exist on gitolite
			ex.On("ExecCommand", "git", time.Duration(1*time.Minute), false, mock.AnythingOfType("[]string"),
				"ls-remote", "ssh://gitolite@code.uber.internal/"+c.gpath, "HEAD",
			).Return("", "FATAL: autocreate denied", fmt.Errorf("1"))

			if c.remoteExists {
				ex.On("ExecCommand", "git", time.Duration(1*time.Minute), false, mock.AnythingOfType("[]string"),
					"ls-remote", c.remoteUrl, "HEAD",
				).Return("", "", nil)
				//successful creation on gitolite
				ex.On("ExecCommand", "ssh", time.Duration(2*time.Minute), true, mock.AnythingOfType("[]string"),
					"gitolite@code.uber.internal", "create", c.mirrorGpath,
				).Return("", "", nil).Once()
			} else {
				ex.On("ExecCommand", "git", time.Duration(1*time.Minute), false, mock.AnythingOfType("[]string"),
					"ls-remote", c.remoteUrl, "HEAD",
				).Return("", "", errors.Errorf("failing remote check"))
			}

			u, err := url.Parse(c.expected)
			if err != nil {
				t.Fatalf("Failed to parse URL %s", c.expected)
			}
			err = assertPanic(t, func() error { return CheckAndMirrorRepo(ex, c.gpath, c.remoteUrl, u) }, !c.remoteExists)
			assert.Nil(t, err)
			ex.AssertExpectations(t)
		}(c)
	}
}

func assertPanic(t *testing.T, f func() error, expectPanic bool) error {
	defer func() {
		if r := recover(); r == nil {
			if expectPanic {
				t.Errorf("The code did not panic when it was expected")
			}
		} else {
			if !expectPanic {
				t.Errorf("The code paniced when it was not expected")
			}
		}
	}()
	return f()
}
