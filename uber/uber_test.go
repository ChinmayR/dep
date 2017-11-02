// testing the uber-specific hacks since 2016
package vcs

import (
	"log"
	"os"
	"testing"

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
			given:      "https://gopkg.in/validator.v2",
			expected:   "https://gopkg.uberinternal.com/validator.v2",
			autocreate: true,
		},
		{
			given:    "https://gopkg.in/validator.v2",
			expected: "gitolite@code.uber.internal:github/go-validator/validator",
			redirect: true,
		},
		{
			given:    "https://gopkg.in/go-validator/validator.v2",
			expected: "gitolite@code.uber.internal:github/go-validator/validator",
			redirect: true,
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if c.redirect {
				defer os.Setenv(uberGopkgRedirectEnv, os.Getenv(uberGopkgRedirectEnv))
				os.Setenv(uberGopkgRedirectEnv, "yapp")
			}

			if !c.autocreate {
				restore := setDisableGitoliteAutocreate("yes")
				defer restore()
			}

			r := repoFromCase(c)

			got := r.remoteOrGitolite()

			assert.Equal(t, c.expected, got)
		}(c)
	}
}

func TestUber_IsGitolite(t *testing.T) {
	cases := []repoTestCase{
		{
			given:      "https://github.com/Masterminds/glide",
			expected:   "gitolite@code.uber.internal:github/Masterminds/glide",
			autocreate: true,
		},
		{
			given:    "golang.org/x/net",
			expected: "golang.org/x/net",
		},
		{
			given:    "https://code.uber.internal/go-common.git",
			expected: "gitolite@code.uber.internal:go-common.git",
		},
		{
			given:    "https://code.uber.internal/rt/filter.git",
			expected: "gitolite@code.uber.internal:rt/filter.git",
		},
		{
			given:    "https://golang.org/x/net",
			expected: "gitolite@code.uber.internal:googlesource/net",
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if !c.autocreate {
				restore := setDisableGitoliteAutocreate("yes")
				defer restore()
			}
			r := repoFromCase(c)
			got := r.remoteOrGitolite()

			assert.Equal(t, c.expected, got)
		}(c)
	}
}

func repoFromCase(c repoTestCase) *GitRepo {
	return &GitRepo{
		base: base{
			Logger: log.New(os.Stdout, ">test ", 0),
			remote: c.given,
		},
	}
}

func setDisableGitoliteAutocreate(val string) func() {
	old := os.Getenv(uberDisableGitoliteAutocreation)
	os.Setenv(uberDisableGitoliteAutocreation, val)

	return func() {
		os.Setenv(uberDisableGitoliteAutocreation, old)
	}
}
