package uber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUber_FiltersInvalidCharacters(t *testing.T) {
	root := "/home/user/go/test+repo,name=with invalid:characters|all\nover.git"
	repo := getRepoTagFriendlyNameFromCWD(root)
	assert.Equal(t, "test-repo-name-with-invalid-characters-all-over", repo)
}

func TestUber_FiltersInvalidCharactersWithGitSuffix(t *testing.T) {
	root := "/home/user/go/test+repo,name=with invalid:characters|all\nover.git"
	repo := getRepoTagFriendlyNameFromCWD(root)
	assert.Equal(t, "test-repo-name-with-invalid-characters-all-over", repo)
}
