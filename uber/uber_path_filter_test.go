package uber

import (
	"testing"
)

type pathTestcase struct {
	path         string
	isUberGoPath bool
}

var pathsCases = []pathTestcase{
	{
		path:         "github.com/uber-go/atomic",
		isUberGoPath: true,
	},
	{
		path:         "github.com/golang/dep",
		isUberGoPath: false,
	},
	{
		path:         "github.com/yarpc/metrics",
		isUberGoPath: true,
	},
	{
		path:         "github.com/thriftrw/thriftrw-go",
		isUberGoPath: true,
	},
	{
		path:         "github.com/uber/go-torch",
		isUberGoPath: true,
	},
	{
		path:         "github.com/test/testify",
		isUberGoPath: false,
	},
	{
		path:         "github.com/yarpc/yarpc-go",
		isUberGoPath: true,
	},
	{
		path:         "github.com/thisIsNot/UberGoPath",
		isUberGoPath: false,
	},
}

func TestIsUberOrgPath(t *testing.T) {
	for _, pathCase := range pathsCases {
		t.Run(pathCase.path, func(t *testing.T) {
			t.Parallel()

			want := pathCase.isUberGoPath
			got := IsGoUberOrgPath(pathCase.path)

			if got != want {
				t.Errorf("Path did not match expectation for go.uber.org path %s:\n\t(GOT) %t\n\t(WNT) %t",
					pathCase.path, got, want)
			}
		})
	}
}
