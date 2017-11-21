// testing the uber-specific hacks since 2016
package uber

import (
	"fmt"
	"testing"

	"github.com/golang/dep/uber/mocks"
	"github.com/stretchr/testify/assert"
)

type repoTestCase struct {
	given, expected string
	redirect        bool
	autocreate      bool
}

func TestUber_IsGopkg(t *testing.T) {
	cases := []repoTestCase{
		{
			given:      "gopkg.in/validator.v2",
			expected:   "https://gopkg.uberinternal.com/validator.v2",
			autocreate: false,
		},
		{
			given:    "gopkg.in/validator.v2",
			expected: "ssh://gitolite@code.uber.internal/github/go-validator/validator",
			redirect: true,
		},
		{
			given:    "gopkg.in/go-validator/validator.v2",
			expected: "ssh://gitolite@code.uber.internal/github/go-validator/validator",
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
			got, err := GetGitoliteUrlForRewriter(c.given, "gopkg.in")

			assert.Nil(t, err)
			assert.Equal(t, c.expected, got.String())
		}(c)
	}
}

func TestUber_IsGitoliteForGitolite(t *testing.T) {
	cases := []repoTestCase{
		{
			given:    "code.uber.internal/go-common.git",
			expected: "ssh://gitolite@code.uber.internal/go-common.git",
		},
		{
			given:    "code.uber.internal/go-common.git/blah",
			expected: "ssh://gitolite@code.uber.internal/go-common.git",
		},
		{
			given:    "code.uber.internal/rt/filter.git",
			expected: "ssh://gitolite@code.uber.internal/rt/filter.git",
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			got, err := GetGitoliteUrlForRewriter(c.given, "code.uber.internal")

			assert.Nil(t, err)
			assert.Equal(t, c.expected, got.String())
		}(c)
	}
}

func TestUber_IsGitoliteForGolang(t *testing.T) {
	cases := []repoTestCase{
		{
			given:    "golang.org/x/net",
			expected: "ssh://gitolite@code.uber.internal/googlesource/net",
		},
		{
			given:    "golang.org/x/net/ipv4",
			expected: "ssh://gitolite@code.uber.internal/googlesource/net",
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			got, err := GetGitoliteUrlForRewriter(c.given, "golang.org")

			assert.Nil(t, err)
			assert.Equal(t, c.expected, got.String())
		}(c)
	}
}

func TestUber_IsGitoliteForGithub(t *testing.T) {
	cases := []repoTestCase{
		{
			given:      "github.com/Masterminds/glide",
			expected:   "ssh://gitolite@code.uber.internal/github/Masterminds/glide",
			autocreate: true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if !c.autocreate {
				defer SetEnvVar(UberDisableGitoliteAutocreation, "yes")()
			}
			got, err := GetGitoliteUrlForRewriter(c.given, "github.com")

			assert.Nil(t, err)
			assert.Equal(t, c.expected, got.String())
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
			expected:     "https://gopkg.uberinternal.com/repo.v0",
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
			got, err := useRewriterWithExecutor(c.importPath, c.rewritername, ex)
			assert.Nil(t, err)
			assert.Equal(t, c.expected, got.String())
			ex.AssertExpectations(t)
		}(c)
	}
}
