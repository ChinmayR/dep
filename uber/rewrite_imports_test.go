package uber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var testcases = map[string]struct {
	path     string
	wantPath string
}{
	"non sirupsen path returns nothing": {
		path:     "github.com/golang/dep/gps",
		wantPath: "github.com/golang/dep/gps",
	},
	"sirupsen path returns same": {
		path:     "github.com/sirupsen/anyPkg",
		wantPath: "github.com/sirupsen/anyPkg",
	},
	"Sirupsen path returns same": {
		path:     "github.com/Sirupsen/anyPkg",
		wantPath: "github.com/sirupsen/anyPkg",
	},
}

func TestRewriteSirupsenImports(t *testing.T) {
	for _, tc := range testcases {
		gotPath := RewriteSirupsenImports(tc.path)

		assert.Equal(t, tc.wantPath, gotPath, "did not get expected path")
	}
}
