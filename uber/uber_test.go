// testing the uber-specific hacks since 2016
package uber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"fmt"
	"net/url"
	"github.com/golang/dep/uber/mocks"
)

type repoTestCase struct {
	given, expUrl, expGpath, expRemote, expGitoliteUrl string
	redirect      bool
	autocreate    bool
}

func TestUber_IsGopkg(t *testing.T) {
	cases := []repoTestCase{
		{
			given:      "gopkg.in/validator.v2",
			expUrl:     "https://gopkg.uberinternal.com/validator.v2",
			expGpath:     "github/go-validator/validator",
			expRemote:     "git@github.com:go-validator/validator",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			autocreate: false,
		},
		{
			given:    "gopkg.in/validator.v2",
			expUrl:   "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			expGpath:     "",
			expRemote:     "",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			redirect: true,
		},
		{
			given:    "gopkg.in/go-validator/validator.v2",
			expUrl:   "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			expGpath:     "",
			expRemote:     "",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			redirect: true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if c.redirect {
				defer SetEnvVar(UberGopkgRedirectEnv, "yes")()
			}

			if !c.autocreate {
				defer SetEnvVar(UberDisableGitoliteAutocreation, "yes")()
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
			given:  "code.uber.internal/go-common.git",
			expUrl: "ssh://gitolite@code.uber.internal/go-common.git",
			expGpath:     "",
			expRemote:     "",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/go-common.git",
		},
		{
			given:  "code.uber.internal/go-common.git/blah",
			expUrl: "ssh://gitolite@code.uber.internal/go-common.git",
			expGpath:     "",
			expRemote:     "",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/go-common.git",
		},
		{
			given:  "code.uber.internal/rt/filter.git",
			expUrl: "ssh://gitolite@code.uber.internal/rt/filter.git",
			expGpath:     "",
			expRemote:     "",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/rt/filter.git",
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
			given:  "golang.org/x/net",
			expUrl: "ssh://gitolite@code.uber.internal/googlesource/net",
			expGpath:     "googlesource/net",
			expRemote:     "https://go.googlesource.com/net",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/googlesource/net",
		},
		{
			given:  "golang.org/x/net/ipv4",
			expUrl: "ssh://gitolite@code.uber.internal/googlesource/net",
			expGpath:     "googlesource/net",
			expRemote:     "https://go.googlesource.com/net",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/googlesource/net",
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
			given:      "github.com/Masterminds/glide",
			expUrl:     "ssh://gitolite@code.uber.internal/github/Masterminds/glide",
			expGpath:     "github/Masterminds/glide",
			expRemote:     "git@github.com:Masterminds/glide",
			expGitoliteUrl:     "ssh://gitolite@code.uber.internal/github/Masterminds/glide",
			autocreate: true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if !c.autocreate {
				defer SetEnvVar(UberDisableGitoliteAutocreation, "yes")()
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

func TestUber_MirrorsToGitolite(t *testing.T) {

	type mirrorTestCase struct {
		importPath   string
		remoteUrl    string
		gpath        string // the repo path on gitolite: code.uber.internal/<gpath> ex: github/user/repo or googlesource/net
		rewritername string
		expected     string
	}

	cases := []mirrorTestCase{
		{
			importPath:   "github.com/test/repo",
			remoteUrl:    "git@github.com:test/repo",
			gpath:        "github/test/repo",
			rewritername: "github.com",
			expected:     "ssh://gitolite@code.uber.internal/github/test/repo",
		},
		{
			importPath:   "gopkg.in/repo.v0",
			remoteUrl:    "git@github.com:go-repo/repo",
			gpath:        "github/go-repo/repo",
			rewritername: "gopkg.in",
			expected:     "ssh://gitolite@code.uber.internal/github/go-repo/repo",
		},
		{
			importPath:   "golang.org/x/repo",
			remoteUrl:    "https://go.googlesource.com/repo",
			gpath:        "googlesource/repo",
			rewritername: "golang.org",
			expected:     "ssh://gitolite@code.uber.internal/googlesource/repo",
		},
	}

	for _, c := range cases {
		func(c mirrorTestCase) {
			ex := &mocks.ExecutorInterface{}
			//does not exist on gitolite
			ex.On("ExecCommand", "git",
				"ls-remote", "ssh://gitolite@code.uber.internal/"+c.gpath, "HEAD",
			).Return("", "FATAL: autocreate denied", fmt.Errorf("1"))
			//remote does exist
			ex.On("ExecCommand", "git",
				"ls-remote", c.remoteUrl, "HEAD",
			).Return("", "", nil)
			//successful creation on gitolite
			ex.On("ExecCommand", "ssh",
				"gitolite@code.uber.internal", "create", c.gpath,
			).Return("", "", nil)
			u, err := url.Parse(c.expected)
			if err != nil {
				t.Fatalf("Failed to parse URL %s", c.expected)
			}
			err = CheckAndMirrorRepo(ex, c.gpath, c.remoteUrl, u)
			assert.Nil(t, err)
			ex.AssertExpectations(t)
		}(c)
	}
}
