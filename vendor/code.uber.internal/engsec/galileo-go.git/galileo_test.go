package galileo_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	. "code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/galileotest"
	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewGalileo(t *testing.T) {
	t.Run("without ServiceName", func(t *testing.T) {
		var cfg Configuration
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Configuration must have ServiceName parameter set")
	})

	t.Run("disabled without ServiceName", func(t *testing.T) {
		cfg := Configuration{Disabled: true}
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Configuration must have ServiceName parameter set")
	})

	t.Run("without tracer", func(t *testing.T) {
		cfg := Configuration{ServiceName: "foo"}
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "jaeger must be initialized before calling galileo")
	})

	t.Run("disabled without tracer", func(t *testing.T) {
		cfg := Configuration{ServiceName: "foo", Disabled: true}
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "jaeger must be initialized before calling galileo")
	})

	t.Run("with NoopTracer", func(t *testing.T) {
		cfg := Configuration{ServiceName: "foo", Tracer: opentracing.NoopTracer{}}
		_, err := Create(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "jaeger must be initialized before calling galileo")
	})
}

func TestDisabled(t *testing.T) {
	type ctxKey string

	cfg := Configuration{
		ServiceName: "foo",
		Disabled:    true,
		Tracer:      mocktracer.New(),
	}

	g, err := Create(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, g)

	wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
		handlers.SetupHandlers(r, handlerCfg)

		ctx := context.WithValue(context.Background(), ctxKey("key"), "value")
		outCtx, err := g.AuthenticateOut(ctx, "foo", "EVERYONE")
		assert.NoError(t, err, "calling authenticate out when disabled shouldn't error")
		assert.NotNil(t, outCtx, "returned context must not be nil")
		assert.Equal(t, ctx, outCtx, "disabled galileo should not modify context")
	})
}

// TestAuthenticateOut covers only well formed input parameters.
func TestAuthenticateOut(t *testing.T) {
	name := "wonkaSample:foober"

	var galileoVars = []struct {
		descr         string // describes the test case
		ctxClaim      string
		explicitClaim string
		err           error
	}{
		{descr: "simple"},
		{descr: "context claim allowed", ctxClaim: name},
		{descr: "context claim rejected", explicitClaim: "impossible-claim",
			// Assertions  on exact error messages are brittle, but,
			// I currently have no other way to compare zap logs.
			err: errors.New(`error requesting claim: error from /claim/v2: 403 error from server: {"result":"REJECTED_CLAIM_NO_ACCESS"}`),
		},
		{descr: "explicit claim ", explicitClaim: "EVERYONE"},
	}

	for _, m := range galileoVars {
		t.Run(m.descr, func(t *testing.T) {
			wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
				handlers.SetupHandlers(r, handlerCfg)

				obs, logs := observer.New(zap.DebugLevel)
				logger := zap.New(obs)
				metrics := tally.NewTestScope("", map[string]string{})
				tracer, ctx, _ := testhelper.SetupContext()

				privKey := wonkatestdata.PrivateKey()
				testhelper.EnrollEntity(ctx, t, handlerCfg.DB, name, privKey)

				cfg := Configuration{
					ServiceName:       name,
					PrivateKeyPath:    testhelper.PrivatePemFromKey(privKey),
					AllowedEntities:   []string{name},
					EnforcePercentage: 1,
					Logger:            logger,
					Metrics:           metrics,
					Tracer:            tracer,
				}

				g, err := CreateWithContext(ctx, cfg)
				require.NoError(t, err, "galileo creation should succeed")
				require.NotNil(t, g, "galileo shouldn't be nil")

				// Discard logs written by CreateWithContext because it is
				// not the system under test.
				_ = logs.TakeAll()

				expectedClaim := "" // expected explicit claim
				claimArgs := []interface{}{}
				if m.ctxClaim != "" {
					ctx = WithClaim(ctx, m.ctxClaim)
					expectedClaim = m.ctxClaim
				}
				if m.explicitClaim != "" {
					expectedClaim = m.explicitClaim
					// Passing empty string to AuthenticateOut
					// overrides the ctxClaim and spoils out test.
					claimArgs = append(claimArgs, m.explicitClaim)
				}

				authedCtx, err := g.AuthenticateOut(ctx, name, claimArgs...)
				assert.NotEqual(t, ctx, authedCtx, "context should be modified")
				authedSpan := opentracing.SpanFromContext(authedCtx).(*mocktracer.MockSpan)

				expectedZapLevel := zapcore.DebugLevel
				expectedZapMessage := "authenticate out succeeded"
				expectBaggage := true
				if m.err != nil {
					expectedZapLevel = zapcore.WarnLevel
					expectedZapMessage = "authenticate out failed"
					expectBaggage = false
				}
				testhelper.AssertZapLog(t, logs, expectedZapLevel, expectedZapMessage,
					[]zapcore.Field{
						zap.Namespace("galileo"),
						zap.String("entity", name),
						zap.String("version", internal.LibraryVersion()),
						zap.Error(m.err), // ok when err is nil
						zap.Bool("has_baggage", expectBaggage),
						zap.String("destination", name),
						zap.String("claim", expectedClaim),
						jaegerzap.Trace(authedCtx),
					})

				testhelper.AssertSpanFieldsLogged(t, authedSpan,
					testhelper.ExpectedOutboundSpanFields(expectBaggage, name, name),
				)

				testhelper.AssertM3Counter(t, metrics, "out", 1, map[string]string{
					"component":      "galileo",
					"host":           "global",
					"metricsversion": telemetry.MetricsVersion,
					"entity":         telemetry.SanitizeEntityName(name),
					"destination":    name,
					"has_baggage":    strconv.FormatBool(expectBaggage),
				})

				// T1314721 always succeed, even without auth baggage.
				require.NoError(t, err, "AuthenticateOut should succeed")

				inErr := g.AuthenticateIn(authedCtx)
				if m.err == nil {
					require.NoError(t, inErr, "AuthenticateIn should succeed")
				} else {
					// request with
					require.Error(t, inErr, "Request without token should fail AuthenticateIn")
				}
			})
		})
	}
}

// TestAuthenticateOutCallerError covers various malformed input to
// AuthenticateOut.
func TestAuthenticateOutCallerError(t *testing.T) {
	name := "wonkaSample:foober"

	var galileoVars = []struct {
		descr         string // describes the test case
		disabled      bool
		ctxClaim      string
		explicitClaim []interface{}
		noDest        bool
		errMsg        string
	}{
		{
			// Verifies that the explicit claim(s) provided via `AuthenticateOut` takes precedence over the `WithClaim` result.
			descr:         "explicit claim argument preferred over context",
			ctxClaim:      "AD:engsec",
			explicitClaim: []interface{}{"AD:engineering", "EVERYTHING"},
			errMsg:        "only one explicit claim is supported",
		},
		{descr: "no destination", noDest: true, errMsg: "no destination"},
		{descr: "disabled no destination", disabled: true, noDest: true, errMsg: "no destination"},
		{descr: "too many explicit claims", explicitClaim: []interface{}{"one-fish", "two-fish"}, errMsg: "only one explicit claim is supported"},
		{descr: "disabled too many explicit claims", disabled: true, explicitClaim: []interface{}{"one-fish", "two-fish"}, errMsg: "only one explicit claim is supported"},
	}

	for _, m := range galileoVars {
		t.Run(m.descr, func(t *testing.T) {
			wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
				handlers.SetupHandlers(r, handlerCfg)

				obs, logs := observer.New(zap.DebugLevel)
				logger := zap.New(obs)
				metrics := tally.NewTestScope("", map[string]string{})
				tracer, ctx, span := testhelper.SetupContext()

				privKey := wonkatestdata.PrivateKey()
				testhelper.EnrollEntity(ctx, t, handlerCfg.DB, name, privKey)

				cfg := Configuration{
					ServiceName:     name,
					Disabled:        m.disabled,
					PrivateKeyPath:  testhelper.PrivatePemFromKey(privKey),
					AllowedEntities: []string{name},
					Logger:          logger,
					Metrics:         metrics,
					Tracer:          tracer,
				}

				g, err := CreateWithContext(ctx, cfg)
				require.NoError(t, err, "galileo creation should succeed")
				require.NotNil(t, g, "galileo shouldn't be nil")
				// Discard logs written by CreateWithContext because it is
				// not the system under test.
				_ = logs.TakeAll()

				if m.ctxClaim != "" {
					ctx = WithClaim(ctx, m.ctxClaim)
				}

				dest := name
				if m.noDest {
					dest = ""
				}

				outCtx, err := g.AuthenticateOut(ctx, dest, m.explicitClaim...)
				assert.Equal(t, ctx, outCtx, "context should not be modified")

				expectedZapLevel := zapcore.ErrorLevel
				expectedZapMessage := "AuthenticateOut caller error"
				testhelper.AssertZapLog(t, logs, expectedZapLevel, expectedZapMessage,
					[]zapcore.Field{
						zap.Namespace("galileo"),
						zap.String("entity", name),
						zap.String("version", internal.LibraryVersion()),
						zap.Error(err),
						jaegerzap.Trace(ctx),
					})
				testhelper.AssertNoSpanFieldsLogged(t, span)
				testhelper.AssertNoM3Counter(t, metrics, "out")
				require.Error(t, err, "AuthenticateOut should error")
				assert.Contains(t, err.Error(), m.errMsg)
			})
		})
	}
}

func TestAuthenticateInWithDerelicts(t *testing.T) {
	name := "server-under-test"
	obs, logs := observer.New(zap.DebugLevel)
	logger := zap.New(obs)
	metrics := tally.NewTestScope("", map[string]string{})
	tracer, ctx, span := testhelper.SetupContext()

	galileotest.WithServerGalileo(t, name, func(g Galileo) {
		time.Sleep(1 * time.Second) // Wonka client loads derelict list asynchronously

		t.Run("globally derelict", func(t *testing.T) {
			err := g.AuthenticateIn(ctx, CallerName("crufty-derelict-service"))
			assert.NoError(t, err, "globally derelict entities should not require Wonka tokens")
		})
	},
		galileotest.Logger(logger),
		galileotest.Metrics(metrics),
		galileotest.Tracer(tracer),
		galileotest.GlobalDerelictEntities("crufty-derelict-service"),
		galileotest.EnrolledEntities(name),
	)

	expectedErr := internal.ErrNoToken
	expectedStatus := telemetry.StatusNotEnforced

	testhelper.AssertZapLog(t, logs, zapcore.InfoLevel,
		"allowing unauthenticated request",
		[]zapcore.Field{
			zap.Namespace("galileo"),
			zap.String("entity", name),
			zap.String("version", internal.LibraryVersion()),
			zap.Error(expectedErr),
			zap.Skip(), // destination
			zap.Skip(), // remote_entity
			zap.Bool("has_baggage", false),
			zap.Bool("is_derelict", true),
			zap.Float64("enforce_percentage", 1),
			zap.String("allowed", expectedStatus.String()),
			zap.String("unauthorized_reason", "no_token"),
			jaegerzap.Trace(ctx),
		})

	testhelper.AssertSpanFieldsLogged(
		t, span,
		testhelper.ExpectedInboundSpanFields(
			false, 1, expectedStatus.Int(),
			"", "", // destination, remote_entity
		))

	testhelper.AssertM3Counter(t, metrics, "in", 1, map[string]string{
		"metricsversion":      telemetry.MetricsVersion,
		"component":           "galileo",
		"host":                "global",
		"entity":              name,
		"has_baggage":         "false",
		"is_derelict":         "true",
		"allowed":             expectedStatus.String(),
		"unauthorized_reason": "no_token",
	})
}
