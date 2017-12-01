// Package galileotest provides utilites for testing Galileo integration in
// consumers such as galileofx; and xtchannel and xhttp from go-common.
package galileotest

import (
	context "context"
	"crypto/x509"
	"encoding/base64"
	"path"
	"testing"

	"code.uber.internal/engsec/galileo-go.git"
	wonka "code.uber.internal/engsec/wonka-go.git"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	opentracing "github.com/opentracing/opentracing-go"
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

	var o galileoOptions
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

		cfg := galileo.Configuration{
			ServiceName:       name,
			PrivateKeyPath:    privatePem,
			AllowedEntities:   append(o.allowedEntities, name),
			EnforcePercentage: 1.0,
			Metrics:           tally.NoopScope,
			Logger:            zap.NewNop(),
			Tracer:            o.tracer,
		}

		g, err := galileo.Create(cfg)
		require.NoError(t, err, "failed to set up Galileo")

		f(g)
	})
}

// WithClientGalileo builds a new Galileo client with the given name.
func WithClientGalileo(t testing.TB, name string, f func(galileo.Galileo), opts ...GalileoOption) {
	withGalileo(t, name, f, opts...)
}

// WithServerGalileo sets up a fake Wonka server and provides a Galileo
// instance for that server.
//
// Any number of Galileo instances for clients may be created inside the
// callback with the WithClientGalileo call.
func WithServerGalileo(t testing.TB, name string, f func(galileo.Galileo), opts ...GalileoOption) {
	if _insideWonkaMaster.Swap(true) {
		t.Fatalf("WithServerGalileo calls cannot be nested")
	}
	defer _insideWonkaMaster.Store(false)

	var o galileoOptions
	for _, opt := range opts {
		opt(&o)
	}

	wonkatestdata.WithWonkaMaster(name, func(r common.Router, handlerCfg common.HandlerConfig) {
		handlers.SetupHandlers(r, handlerCfg)

		for _, entity := range o.enrolledEntities {
			wonkatestdata.WithTempDir(func(dir string) {
				privPem := path.Join(dir, "wonka_private")
				privKey := wonkatestdata.PrivateKey()
				require.NoError(t, wonkatestdata.WritePrivateKey(privKey, privPem),
					"error writing private key")

				publicKey, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
				require.NoError(t, err, "failed to marshal public key")

				e := wonka.Entity{
					EntityName:   entity,
					PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
					ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(privKey),
				}

				require.NoError(
					t,
					handlerCfg.DB.Create(context.Background(), &e),
					"failed to enroll %q", entity,
				)
			})
		}
		withGalileo(t, name, f, opts...)
	})
}

type galileoOptions struct {
	allowedEntities  []string
	enrolledEntities []string
	tracer           opentracing.Tracer
}

// GalileoOption allows optional configuration for WithServerGalileo.
type GalileoOption func(*galileoOptions)

// AllowedEntities are the list of entity names that are allowed to make
// requests to server created by WithServerGalileo.
func AllowedEntities(entities ...string) GalileoOption {
	return func(o *galileoOptions) {
		o.allowedEntities = append(o.allowedEntities, entities...)
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

// Tracer provides the option to use the given tracer instead of the global
// tracer.
func Tracer(tracer opentracing.Tracer) GalileoOption {
	return func(o *galileoOptions) {
		o.tracer = tracer
	}
}
