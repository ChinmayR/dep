package galileo

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/atomic"
	"code.uber.internal/engsec/galileo-go.git/internal/claimtools"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSkipDestination(t *testing.T) {
	u := &uberGalileo{
		skippedEntities: make(map[string]skippedEntity, 1),
		skipLock:        &sync.RWMutex{},
		log:             zap.L(),
	}

	require.Equal(t, len(u.skippedEntities), 0, "entities should be empty")

	u.addSkipDest("foober")
	require.Equal(t, len(u.skippedEntities), 1, "entities should have one entity")
	ok := u.shouldSkipDest("foober")
	require.True(t, ok, "should skip")

	e, ok := u.skippedEntities["foober"]
	require.True(t, ok, "foober should be skipped")
	require.Equal(t, initialSkipDuration, e.until, "default skip time")

	e.start = time.Now().Add(-time.Hour)
	u.skippedEntities["foober"] = e
	ok = u.shouldSkipDest("foober")
	require.False(t, ok, "should not skip")

	u.addSkipDest("foober")
	require.Equal(t, len(u.skippedEntities), 1, "entities should have one entity")
	ok = u.shouldSkipDest("foober")
	require.True(t, ok, "should skip")

	e, ok = u.skippedEntities["foober"]
	require.True(t, ok, "should skip")
	require.Equal(t, initialSkipDuration, e.until, "reset skip time")

	u.addSkipDest("foober")
	e, ok = u.skippedEntities["foober"]
	require.True(t, ok, "should skip")
	require.Equal(t, 2*initialSkipDuration, e.until, "2x default skip time")

	e.until = 2 * maxSkipDuration
	u.skippedEntities["foober"] = e

	u.addSkipDest("foober")
	e, ok = u.skippedEntities["foober"]
	require.True(t, ok, "should skip")
	require.Equal(t, maxSkipDuration, e.until, "max skip time")
}

// TestClaimUsableUnsignedClaim covers the pathological case where an unsigned
// claim is added to the cache. This should never actually occur.
func TestClaimUsableUnsignedClaim(t *testing.T) {
	now := time.Now()
	claim := &wonka.Claim{
		EntityName:  "TestClaimUsable",
		ValidAfter:  now.Add(-10 * time.Minute).Unix(),
		ValidBefore: now.Add(10 * time.Minute).Unix(),
		Claims:      []string{wonka.EveryEntity},
		Destination: wonka.NullEntity,
	}

	err := claimUsable(claim, "")
	assert.NoError(t, err, "unsigned claim token should be usable")
}

func TestClaimUsable(t *testing.T) {
	now := time.Now()

	var testVars = []struct {
		descr    string             // describes the test case
		errMsg   string             // expected error message. Leave empty if you expect no error.
		claimReq string             // explicity required claim
		before   func(*wonka.Claim) // Modify the claim before the test
	}{
		{descr: "allow any claims"},
		{descr: "explicit required claim", claimReq: "probable-claim", before: func(c *wonka.Claim) { c.Claims = append(c.Claims, "probable-claim") }},
		{descr: "require ungranted claim", errMsg: "claim token does not grant \"wildly-improbable-claim\"", claimReq: "wildly-improbable-claim"},
		{descr: "already expired", errMsg: "claim token will expire soon", before: func(c *wonka.Claim) { c.ValidBefore = now.Add(-5 * time.Minute).Unix() }},
		{descr: "expiring soon", errMsg: "claim token will expire soon", before: func(c *wonka.Claim) { c.ValidBefore = now.Add(2 * time.Minute).Unix() }},
	}

	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(claim *wonka.Claim, _ string) {
				err := claimUsable(claim, m.claimReq)
				if m.errMsg == "" {
					require.NoError(t, err, "should be usable")
				} else {
					require.Error(t, err, "should not be usable")
					assert.Contains(t, err.Error(), m.errMsg, "not the error we expected")
				}
			},
				testhelper.Destination(wonka.NullEntity),
				m.before, // potentially modify claim for this test case
			)
		})
	}
}

func newTestGalileo() *uberGalileo {
	return &uberGalileo{
		metrics: tally.NoopScope,
		log:     zap.L(),
		tracer:  mocktracer.New(),
	}
}

func TestAuthenticateInDisabled(t *testing.T) {
	u := newTestGalileo()
	u.disabled = true
	err := u.AuthenticateIn(context.Background())
	require.NoError(t, err)
}

func TestAuthenticateInInvalidAllowedParameter(t *testing.T) {
	u := newTestGalileo()
	err := u.AuthenticateIn(context.Background(), 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected argument of type int")
}

func TestDoAuthenticateInWithoutClaim(t *testing.T) {
	// It is brittle to make assertions based on exact error messages.
	// However, I currently have no other way to compare zap logs.
	expectedErr := internal.ErrNoToken
	enforcements := []float64{0, 1}
	for _, enforcement := range enforcements {
		t.Run(fmt.Sprintf("enforce_percentage=%0.1f", enforcement), func(t *testing.T) {
			obs, logs := observer.New(zap.DebugLevel)
			logger := zap.New(obs)
			metrics := tally.NewTestScope("", map[string]string{})
			tracer, ctx, span := testhelper.SetupContext()

			u := &uberGalileo{
				enforcePercentage: atomic.NewFloat64(enforcement),
				log:               logger,
				metrics:           metrics,
				tracer:            tracer,
				inboundClaimCache: claimtools.DisabledCache,
			}

			err := u.doAuthenticateIn(ctx, validationConfiguration{})

			expectedZapMessage := "allowing unauthenticated request"
			expectedStatus := telemetry.StatusNotEnforced
			if enforcement == 0 {
				require.NoError(t, err)
			} else {
				expectedZapMessage = "denying unauthenticated request"
				expectedStatus = telemetry.StatusDenied

				require.Error(t, err)
				assert.Equal(t, expectedErr, err, "unexpected error")
			}

			testhelper.AssertZapLog(t, logs, zapcore.InfoLevel, expectedZapMessage,
				[]zapcore.Field{
					zap.Error(expectedErr),
					zap.Skip(), // destination
					zap.Skip(), // remote_entity
					zap.Bool("has_baggage", false),
					zap.Bool("is_derelict", false),
					zap.Float64("enforce_percentage", enforcement),
					zap.String("allowed", expectedStatus.String()),
					zap.String("unauthorized_reason", "no_token"),
					jaegerzap.Trace(ctx),
				})

			testhelper.AssertSpanFieldsLogged(
				t, span,
				testhelper.ExpectedInboundSpanFields(
					false, enforcement, expectedStatus.Int(),
					"", "", // destination, remote_entity
				))

			testhelper.AssertM3Counter(t, metrics, "in", 1, map[string]string{
				"has_baggage":         "false",
				"is_derelict":         "false",
				"allowed":             expectedStatus.String(),
				"unauthorized_reason": "no_token",
			})
		})
	}
}

func TestDoAuthenticateInWithMalformedClaim(t *testing.T) {
	// It is brittle to make assertions based on exact error messages.
	// However, I currently have no other way to compare zap logs.
	expectedErr := internal.NewMalformedTokenError("base64 unmarshalling claim token: illegal base64 data at input byte 3")
	enforcements := []float64{0, 1}
	for _, enforcement := range enforcements {
		t.Run(fmt.Sprintf("enforce_percentage=%0.1f", enforcement), func(t *testing.T) {
			obs, logs := observer.New(zap.DebugLevel)
			logger := zap.New(obs)
			metrics := tally.NewTestScope("", map[string]string{})
			tracer, ctx, span := testhelper.SetupContext()

			u := &uberGalileo{
				enforcePercentage: atomic.NewFloat64(enforcement),
				log:               logger,
				metrics:           metrics,
				tracer:            tracer,
				inboundClaimCache: claimtools.DisabledCache,
			}

			contexthelper.SetBaggage(span, "not-a-real-claim-token")

			err := u.doAuthenticateIn(ctx, validationConfiguration{})

			expectedZapMessage := "allowing unauthenticated request"
			expectedStatus := telemetry.StatusNotEnforced
			if enforcement == 0 {
				assert.NoError(t, err)
			} else {
				expectedZapMessage = "denying unauthenticated request"
				expectedStatus = telemetry.StatusDenied

				require.Error(t, err)
				assert.Equal(t, expectedErr, err, "unexpected error")
			}

			testhelper.AssertZapLog(t, logs, zapcore.InfoLevel, expectedZapMessage,
				[]zapcore.Field{
					zap.Error(expectedErr),
					zap.Skip(), // destination
					zap.Skip(), // remote_entity
					zap.Bool("has_baggage", true),
					zap.Bool("is_derelict", false),
					zap.Float64("enforce_percentage", enforcement),
					zap.String("allowed", expectedStatus.String()),
					zap.String("unauthorized_reason", "malformed_token"),
					jaegerzap.Trace(ctx),
				})

			testhelper.AssertSpanFieldsLogged(
				t, span,
				testhelper.ExpectedInboundSpanFields(
					true, enforcement, expectedStatus.Int(),
					"", "", // destination, remote_entity
				))

			testhelper.AssertM3Counter(t, metrics, "in", 1, map[string]string{
				"has_baggage":         "true",
				"is_derelict":         "false",
				"allowed":             expectedStatus.String(),
				"unauthorized_reason": "malformed_token",
			})
		})
	}
}

func TestDoAuthenticateInSucceedsWithClaim(t *testing.T) {
	alloweds := [][]string{
		nil,
		{wonka.EveryEntity},
		{wonka.EveryEntity, "test"},
	}
	for _, allowed := range alloweds {
		t.Run(fmt.Sprintf("allowed=%q", allowed), func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(claim *wonka.Claim, claimString string) {
				obs, logs := observer.New(zap.DebugLevel)
				logger := zap.New(obs)
				metrics := tally.NewTestScope("", map[string]string{})
				tracer, ctx, span := testhelper.SetupContext()

				enforcement := float64(1)

				vcfg := validationConfiguration{
					AllowedDestinations: []string{"system-under-test"},
					AllowedEntities:     allowed,
				}

				u := &uberGalileo{
					enforcePercentage: atomic.NewFloat64(enforcement),
					log:               logger,
					metrics:           metrics,
					tracer:            tracer,
					inboundClaimCache: claimtools.DisabledCache,
				}

				contexthelper.SetBaggage(span, claimString)

				err := u.doAuthenticateIn(ctx, vcfg)
				assert.NoError(t, err)

				expectedStatus := telemetry.StatusAllowedAllOK

				testhelper.AssertZapLog(t, logs, zapcore.DebugLevel,
					"request successfully authenticated",
					[]zapcore.Field{
						zap.Error(nil),
						zap.String("destination", claim.Destination),
						zap.String("remote_entity", claim.EntityName),
						zap.Bool("has_baggage", true),
						zap.Bool("is_derelict", false),
						zap.Float64("enforce_percentage", enforcement),
						zap.String("allowed", expectedStatus.String()),
						zap.String("unauthorized_reason", ""),
						jaegerzap.Trace(ctx),
					})

				testhelper.AssertSpanFieldsLogged(
					t, span,
					testhelper.ExpectedInboundSpanFields(
						true, enforcement, expectedStatus.Int(),
						claim.Destination, claim.EntityName,
					))

				testhelper.AssertM3Counter(t, metrics, "in", 1, map[string]string{
					"has_baggage":   "true",
					"is_derelict":   "false",
					"allowed":       expectedStatus.String(),
					"remote_entity": telemetry.SanitizeEntityName(claim.EntityName),
				})

			}, testhelper.Destination("system-under-test"))
		})
	}
}

func TestDoAuthenticateInWithServiceAliases(t *testing.T) {
	var testVars = []struct {
		descr       string
		destination string
		err         internal.InboundAuthenticationError
	}{
		{
			descr:       "first destination in alias list",
			destination: "first-alias",
		},
		{
			descr:       "second destination in alias list",
			destination: "second-alias",
		},
		{
			descr:       "unacceptable destination",
			destination: "not-an-alias",
			err: internal.NewInboundAuthenticationErrorf(
				internal.UnauthorizedWrongDestination, true,
				`not permitted by configuration: claim token destination "not-an-alias" is not among allowed destinations ["first-alias" "second-alias"]`,
			),
		},
	}
	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(claim *wonka.Claim, claimString string) {

				obs, logs := observer.New(zap.DebugLevel)
				logger := zap.New(obs)
				metrics := tally.NewTestScope("", map[string]string{})
				tracer, ctx, span := testhelper.SetupContext()

				enforcement := float64(1)

				vcfg := validationConfiguration{
					AllowedDestinations: []string{"first-alias", "second-alias"},
					AllowedEntities:     []string{wonka.EveryEntity},
				}

				u := &uberGalileo{
					enforcePercentage: atomic.NewFloat64(enforcement),
					log:               logger,
					metrics:           metrics,
					tracer:            tracer,
					inboundClaimCache: claimtools.DisabledCache,
				}

				contexthelper.SetBaggage(span, claimString)

				err := u.doAuthenticateIn(ctx, vcfg)

				expectedZapLevel := zapcore.DebugLevel
				expectedZapMessage := "request successfully authenticated"
				expectedStatus := telemetry.StatusAllowedAllOK
				expectedReason := ""
				expectedM3Tags := map[string]string{
					"has_baggage":   "true",
					"is_derelict":   "false",
					"remote_entity": telemetry.SanitizeEntityName(claim.EntityName),
				}
				if m.err == nil {
					require.NoError(t, err, "doAuthenticateIn should succeed")
				} else {
					assert.Error(t, err, "doAuthenticateIn should fail")

					expectedZapLevel = zapcore.InfoLevel
					expectedZapMessage = "denying unauthenticated request"
					expectedStatus = telemetry.StatusDenied
					expectedReason = internal.UnauthorizedWrongDestination.String()
					expectedM3Tags["unauthorized_reason"] = expectedReason
				}

				testhelper.AssertZapLog(t, logs, expectedZapLevel, expectedZapMessage,
					[]zapcore.Field{
						zap.Error(m.err),
						zap.String("destination", m.destination),
						zap.String("remote_entity", claim.EntityName),
						zap.Bool("has_baggage", true),
						zap.Bool("is_derelict", false),
						zap.Float64("enforce_percentage", enforcement),
						zap.String("allowed", expectedStatus.String()),
						zap.String("unauthorized_reason", expectedReason),
						jaegerzap.Trace(ctx),
					})

				testhelper.AssertSpanFieldsLogged(
					t, span,
					testhelper.ExpectedInboundSpanFields(
						true, enforcement, expectedStatus.Int(),
						m.destination, claim.EntityName,
					))

				expectedM3Tags["allowed"] = expectedStatus.String()
				testhelper.AssertM3Counter(t, metrics, "in", 1, expectedM3Tags)

			}, testhelper.Destination(m.destination))
		})
	}
}

func TestDoAuthenticateInWithCallerName(t *testing.T) {
	serverEntityName := "system-under-test"
	remoteEntityName := "authentic-alice"
	enforcement := float64(1)
	expectedStatus := telemetry.StatusAllowedAllOK

	var testVars = []struct {
		descr      string
		callerName string
	}{
		{
			descr:      "entity name matches caller name",
			callerName: remoteEntityName,
		},
		{
			descr:      "entity name and caller name differ",
			callerName: "eve@example.com",
		},
	}
	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(_ *wonka.Claim, claimString string) {

				obs, logs := observer.New(zap.DebugLevel)
				logger := zap.New(obs)
				metrics := tally.NewTestScope("", map[string]string{})
				tracer, ctx, span := testhelper.SetupContext()

				vcfg := validationConfiguration{
					AllowedDestinations: []string{serverEntityName},
					AllowedEntities:     []string{wonka.EveryEntity},
					CallerName:          m.callerName,
				}

				u := &uberGalileo{
					enforcePercentage: atomic.NewFloat64(enforcement),
					log:               logger,
					metrics:           metrics,
					tracer:            tracer,
					inboundClaimCache: claimtools.DisabledCache,
				}

				contexthelper.SetBaggage(span, claimString)

				err := u.doAuthenticateIn(ctx, vcfg)

				require.NoError(t, err, "doAuthenticateIn should succeed")

				if m.callerName != remoteEntityName {
					testhelper.AssertZapLog(t, logs, zapcore.InfoLevel,
						"remote entity name mismatch",
						[]zapcore.Field{
							zap.String("remote_entity", remoteEntityName),
							zap.String("caller_name", m.callerName),
						},
					)
				}

				testhelper.AssertZapLog(t, logs, zapcore.DebugLevel,
					"request successfully authenticated",
					[]zapcore.Field{
						zap.Skip(), // No error
						zap.String("destination", serverEntityName),
						zap.String("remote_entity", remoteEntityName),
						zap.Bool("has_baggage", true),
						zap.Bool("is_derelict", false),
						zap.Float64("enforce_percentage", enforcement),
						zap.String("allowed", expectedStatus.String()),
						zap.String("unauthorized_reason", ""),
						jaegerzap.Trace(ctx),
					})

				testhelper.AssertSpanFieldsLogged(
					t, span,
					testhelper.ExpectedInboundSpanFields(
						true, enforcement, expectedStatus.Int(),
						serverEntityName, remoteEntityName,
					))

				testhelper.AssertM3Counter(t, metrics, "in", 1,
					map[string]string{
						"has_baggage":   "true",
						"is_derelict":   "false",
						"remote_entity": remoteEntityName,
						"allowed":       expectedStatus.String(),
					})
			},
				testhelper.Destination(serverEntityName),
				testhelper.EntityName(remoteEntityName),
			)
		})
	}
}

func TestDoAuthenticateInWithBadClaim(t *testing.T) {
	// It is brittle to make assertions based on exact error messages.
	// However, I currently have no other way to compare zap logs.
	expectedErr := internal.NewInboundAuthenticationErrorf(
		internal.UnauthorizedNoCommonClaims, true,
		`not permitted by configuration: no common claims between token and configured allowed_entities. allowed_entities=["some-crazy-claim"]; granted_claims=["EVERYONE"]`,
	)
	enforcements := []float64{0, 1}
	for _, enforcement := range enforcements {
		t.Run(fmt.Sprintf("enforce_percentage=%0.1f", enforcement), func(t *testing.T) {
			testhelper.WithSignedClaim(t, func(claim *wonka.Claim, claimString string) {
				obs, logs := observer.New(zap.DebugLevel)
				logger := zap.New(obs)
				metrics := tally.NewTestScope("", map[string]string{})
				tracer, ctx, span := testhelper.SetupContext()

				vcfg := validationConfiguration{
					AllowedDestinations: []string{"some-crazy-service"},
					AllowedEntities:     []string{"some-crazy-claim"},
				}

				u := &uberGalileo{
					enforcePercentage: atomic.NewFloat64(enforcement),
					log:               logger,
					metrics:           metrics,
					tracer:            tracer,
					inboundClaimCache: claimtools.DisabledCache,
				}

				contexthelper.SetBaggage(span, claimString)

				err := u.doAuthenticateIn(ctx, vcfg)

				expectedZapMessage := "allowing unauthenticated request"
				expectedStatus := telemetry.StatusNotEnforced
				if enforcement == 0 {
					require.NoError(t, err)
				} else {
					expectedZapMessage = "denying unauthenticated request"
					expectedStatus = telemetry.StatusDenied

					require.EqualError(t, err, expectedErr.Error())
				}

				testhelper.AssertZapLog(t, logs, zapcore.InfoLevel, expectedZapMessage,
					[]zapcore.Field{
						zap.Error(expectedErr),
						zap.String("destination", claim.Destination),
						zap.String("remote_entity", claim.EntityName),
						zap.Bool("has_baggage", true),
						zap.Bool("is_derelict", false),
						zap.Float64("enforce_percentage", enforcement),
						zap.String("allowed", expectedStatus.String()),
						zap.String("unauthorized_reason", "no_common_claims"),
						jaegerzap.Trace(ctx),
					})

				testhelper.AssertSpanFieldsLogged(
					t, span,
					testhelper.ExpectedInboundSpanFields(
						true, enforcement, expectedStatus.Int(),
						claim.Destination, claim.EntityName,
					))

				testhelper.AssertM3Counter(t, metrics, "in", 1, map[string]string{
					"has_baggage":         "true",
					"is_derelict":         "false",
					"allowed":             expectedStatus.String(),
					"unauthorized_reason": "no_common_claims",
					"remote_entity":       telemetry.SanitizeEntityName(claim.EntityName),
				})

			}, testhelper.Destination("some-crazy-service"))
		})
	}
}

func TestNoEndpointConfig(t *testing.T) {
	u := &uberGalileo{}
	_, err := u.Endpoint("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no configuration for endpoint")
}

func TestEndpointConfig(t *testing.T) {
	fooCfg := EndpointCfg{
		AllowRead:  []string{"bar", "baz"},
		AllowWrite: []string{"bar"},
	}
	u := &uberGalileo{
		endpointCfg: map[string]EndpointCfg{"foo": fooCfg},
	}
	cfg, err := u.Endpoint("foo")
	require.NoError(t, err)
	require.Equal(t, fooCfg, cfg)
}

func BenchmarkAuthenticateInCacheDisabled(b *testing.B) {
	allowed := []string{wonka.EveryEntity}
	testhelper.WithSignedClaim(b, func(claim *wonka.Claim, claimString string) {
		tracer, ctx, span := testhelper.SetupContext()

		vcfg := validationConfiguration{
			AllowedDestinations: []string{"foo"},
			AllowedEntities:     allowed,
		}

		u := &uberGalileo{
			enforcePercentage: atomic.NewFloat64(1),
			log:               zap.L(),
			metrics:           tally.NoopScope,
			tracer:            tracer,
			inboundClaimCache: claimtools.DisabledCache,
		}
		for i := 0; i < b.N; i++ {
			contexthelper.SetBaggage(span, claimString)
			err := u.doAuthenticateIn(ctx, vcfg)
			require.NoError(b, err)
		}
	}, testhelper.Claims(allowed...), testhelper.Destination("foo"))
}

func BenchmarkAuthenticateInCache(b *testing.B) {
	allowed := []string{wonka.EveryEntity}
	testhelper.WithSignedClaim(b, func(claim *wonka.Claim, claimString string) {
		tracer, ctx, span := testhelper.SetupContext()
		inboundClaimCache, err := claimtools.NewInboundCache(claimtools.CacheConfig{})
		require.NoError(b, err)

		vcfg := validationConfiguration{
			AllowedDestinations: []string{"foo"},
			AllowedEntities:     allowed,
		}

		u := &uberGalileo{
			enforcePercentage: atomic.NewFloat64(1),
			log:               zap.L(),
			metrics:           tally.NoopScope,
			tracer:            tracer,
			inboundClaimCache: inboundClaimCache,
		}
		for i := 0; i < b.N; i++ {
			contexthelper.SetBaggage(span, claimString)
			err := u.doAuthenticateIn(ctx, vcfg)
			require.NoError(b, err)
		}
	}, testhelper.Claims(allowed...), testhelper.Destination("foo"))
}

func BenchmarkAuthenticateInCache100Claims(b *testing.B) {
	benchmarkAuthenticateInNClaims(b, 100)
}

func BenchmarkAuthenticateInCache1000Claims(b *testing.B) {
	benchmarkAuthenticateInNClaims(b, 1000)
}

func BenchmarkAuthenticateInCache10000Claims(b *testing.B) {
	benchmarkAuthenticateInNClaims(b, 10000)
}

func benchmarkAuthenticateInNClaims(b *testing.B, numTokens int) {
	privKey := wonkatestdata.ECCKey()
	defer testhelper.SetTempWonkaMasterPublicKey(&privKey.PublicKey)()

	claimTokens := genClaimTokens(b, numTokens)
	tracer, ctx, span := testhelper.SetupContext()
	inboundClaimCache, err := claimtools.NewInboundCache(claimtools.CacheConfig{})
	require.NoError(b, err)

	vcfg := validationConfiguration{
		AllowedDestinations: []string{"foo"},
		AllowedEntities:     []string{wonka.EveryEntity},
	}

	u := &uberGalileo{
		enforcePercentage: atomic.NewFloat64(1),
		log:               zap.L(),
		metrics:           tally.NoopScope,
		tracer:            tracer,
		inboundClaimCache: inboundClaimCache,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		contexthelper.SetBaggage(span, claimTokens[i%numTokens])
		err := u.doAuthenticateIn(ctx, vcfg)
		require.NoError(b, err)
	}
}

func genClaimTokens(b *testing.B, n int) []string {
	tokens := make([]string, n)
	for i := range tokens {
		testhelper.WithSignedClaim(b,
			func(claim *wonka.Claim, claimString string) {
				tokens[i] = claimString
			},
			testhelper.Claims(wonka.EveryEntity),
			testhelper.Destination("foo"),
			testhelper.EntityName(fmt.Sprintf("bar-%d", i)),
		)
	}
	return tokens
}
