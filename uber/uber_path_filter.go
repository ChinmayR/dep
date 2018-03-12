package uber

import (
	"strings"
)

// go.uber.org internal repos lead to an external path of github, and the same external github
// repos lead to an external remote path of github (but an internal path of gitolite repo).
// When we get these overlapping github paths, we end up setting the same remote URL for both.
// The same remote path for both cases is invalid and so we need to ignore the external github
// path in the case when it can also be defined as a go.uber.org path.
var goUberPaths = []string{
	"github.com/uber-go/atomic",
	"github.com/uber-go/automaxprocs",
	"github.com/uber-go/cadence-client",
	"github.com/uber-go/config",
	"github.com/uber-go/dig",
	"github.com/uber-go/fx",
	"github.com/uber-go/goleak",
	"github.com/uber-go/multierr",
	"github.com/yarpc/metrics",
	"github.com/uber-go/protoidl",
	"github.com/uber-go/ratelimit",
	"github.com/uber-go/sally",
	"github.com/thriftrw/thriftrw-go",
	"github.com/uber-go/tools",
	"github.com/uber/go-torch",
	"github.com/yarpc/yarpc-go",
	"github.com/uber-go/zap",
}

func IsGoUberOrgPath(path string) bool {
	for _, goUberPath := range goUberPaths {
		if strings.Contains(path, goUberPath) {
			return true
		}
	}
	return false
}
