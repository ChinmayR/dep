package netmetricsfx

import (
	"testing"

	versionfx "code.uber.internal/go/versionfx.git"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/net/metrics"
)

func TestFxIntegration(t *testing.T) {
	var p struct {
		fx.In

		Scope *metrics.Scope
	}

	app := fxtest.New(t,
		Module,
		fx.Provide(
			func() tally.Scope { return tally.NoopScope },
			func() *versionfx.Reporter { return new(versionfx.Reporter) },
		),
		fx.Extract(&p),
	)
	app.RequireStart().RequireStop()
	require.NotNil(t, p.Scope)
}

func TestFxIntegrationNilParams(t *testing.T) {
	var p struct {
		fx.In

		Scope *metrics.Scope
	}

	app := fxtest.New(t,
		Module,
		fx.Extract(&p),
	)
	app.RequireStart().RequireStop()
	require.NotNil(t, p.Scope)
}
