// testing the uber-specific hacks since 2016
package uber

import (
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
			got := GetGitoliteUrlWithPath(c.given)

			assert.Equal(t, c.expected, got.String())
		}(c)
	}
}

/* --- This will fail until the Golang changes are pushed through
func TestUber_IsGitoliteForGolang(t *testing.T) {
	cases := []repoTestCase{
		{
			given:    "golang.org/x/net",
			expected: "golang.org/x/net",
		},
		{
			given:    "golang.org/x/net",
			expected: "ssh://gitolite@code.uber.internal/googlesource/net",
		},
	}

	for _, c := range cases {
		func(c repoTestCase) {
			if !c.autocreate {
				restore := setDisableGitoliteAutocreate("yes")
				defer restore()
			}
			got, err := GetGitoliteUrlForRewriter(c.given, "golang.org")
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, c.expected, got.String())
		}(c)
	}
}

*/

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
