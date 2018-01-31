// Package galileotest provides utilites for testing Galileo integration in
// consumers such as galileofx; and xtchannel and xhttp from go-common.
package galileotest

import (
	"context"
	"path"
	"testing"
	"time"

	"code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// Ensure MockGalileo still implements Galileo interface.
// If this fails, run `go generate galileotest/gen.go`.
var _ galileo.Galileo = (*MockGalileo)(nil)

// Unfortunately this state has to be tracked globally because environment
// variables are used by Galileo to pick up the Wonka address.
var _insideWonkaMaster = atomic.NewBool(false)

// withGalileo builds a new Galileo client with the given name allowing
// requests from the given entities.
func withGalileo(t testing.TB, name string, f func(galileo.Galileo), opts ...GalileoOption) {
	if !_insideWonkaMaster.Load() {
		t.Fatalf("WithClientGalileo must be called inside a WithServerGalileo callback")
	}

	o := galileoOptions{
		Configuration: galileo.Configuration{
			ServiceName:       name,
			AllowedEntities:   []string{name},
			EnforcePercentage: 1.0,
			Metrics:           tally.NoopScope,
			Logger:            zap.NewNop(),
			Tracer:            mocktracer.New(),
		},
	}
	for _, opt := range opts {
		opt(&o)
	}

	wonkatestdata.WithTempDir(func(dir string) {
		privatePem := path.Join(dir, "private.pem")

		privKey := wonkatestdata.PrivateKey()
		require.NoError(t,
			wonkatestdata.WritePrivateKey(privKey, privatePem),
			"error writing private key",
		)

		o.Configuration.PrivateKeyPath = privatePem

		g, err := galileo.Create(o.Configuration)
		require.NoError(t, err, "failed to set up Galileo")

		f(g)
	})
}

// NewDisabled builds a new disabled Galileo client with the given name and
// with minimal other configuration. Intended for unit tests. You should also
// conider using galileotest.MockGalileo. For integration tests you should use
// WithServerGalileo and WithClientGalileo instead.
func NewDisabled(t testing.TB, name string) galileo.Galileo {
	cfg := galileo.Configuration{
		ServiceName: name,
		Disabled:    true,
		Metrics:     tally.NoopScope,
		Logger:      zap.NewNop(),
		Tracer:      mocktracer.New(),
	}

	g, err := galileo.Create(cfg)
	require.NoError(t, err, "failed to set up Galileo")
	return g
}

// WithClientGalileo builds a new, enabled, Galileo client with the given name.
// Must be called inside WithServerGalileo.
func WithClientGalileo(t testing.TB, name string, f func(galileo.Galileo), opts ...GalileoOption) {
	withGalileo(t, name, f, opts...)
}

// WithServerGalileo sets up a fake Wonka server and provides an enabled Galileo
// instance for that server.
//
// Any number of Galileo instances for clients may be created inside the
// callback with the WithClientGalileo call.
func WithServerGalileo(t testing.TB, name string, f func(galileo.Galileo), opts ...GalileoOption) {
	if _insideWonkaMaster.Swap(true) {
		t.Fatalf("WithServerGalileo calls cannot be nested")
	}
	defer _insideWonkaMaster.Store(false)

	o := galileoOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	wonkatestdata.WithWonkaMaster(name, func(r common.Router, handlerCfg common.HandlerConfig) {
		if len(o.globalDerelictEntities) > 0 {
			derelictUntil := time.Now().Add(48 * time.Hour).Format("2006/01/02")
			handlerCfg.Derelicts = make(map[string]string, len(o.globalDerelictEntities))

			for _, e := range o.globalDerelictEntities {
				handlerCfg.Derelicts[e] = derelictUntil
			}
		}
		handlers.SetupHandlers(r, handlerCfg)

		ctx := context.Background()

		for _, entityName := range o.enrolledEntities {
			testhelper.EnrollEntity(ctx, t, handlerCfg.DB, entityName, wonkatestdata.PrivateKey())
		}
		withGalileo(t, name, f, opts...)
	})
}

type galileoOptions struct {
	galileo.Configuration

	enrolledEntities       []string
	globalDerelictEntities []string
}

// GalileoOption allows optional configuration for WithServerGalileo.
type GalileoOption func(*galileoOptions)

// AllowedEntities are the list of entity names that are allowed to make
// requests to server created by WithServerGalileo.
func AllowedEntities(entities ...string) GalileoOption {
	return func(o *galileoOptions) {
		o.AllowedEntities = append(o.AllowedEntities, entities...)
	}
}

// GlobalDerelictEntities configures Wonkamaster with a list of entity names
// that are allowed to make unauthenticated requests. Wonkamaster serves this
// list to configure every service.
func GlobalDerelictEntities(entities ...string) GalileoOption {
	return func(o *galileoOptions) {
		o.globalDerelictEntities = append(o.globalDerelictEntities, entities...)
	}
}

// EnrolledEntities are the list of entity names that exist in the wonka
// ecosystem defined by WithServerGalileo. This should include all entities you
// use withing your test case.
func EnrolledEntities(entities ...string) GalileoOption {
	return func(o *galileoOptions) {
		o.enrolledEntities = append(o.enrolledEntities, entities...)
	}
}

// Logger configures Galileo with the specified specific logger instead of the
// default noop logger.
func Logger(logger *zap.Logger) GalileoOption {
	return func(o *galileoOptions) {
		o.Configuration.Logger = logger
	}
}

// Metrics configures Galileo with the specified metric scope instead of the
// default noop scope.
func Metrics(metrics tally.Scope) GalileoOption {
	return func(o *galileoOptions) {
		o.Configuration.Metrics = metrics
	}
}

// Tracer provides the option to use the given tracer instead of the global
// tracer.
func Tracer(tracer opentracing.Tracer) GalileoOption {
	return func(o *galileoOptions) {
		o.Configuration.Tracer = tracer
	}
}
