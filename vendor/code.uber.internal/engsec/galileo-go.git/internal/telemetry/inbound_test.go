package telemetry_test

import (
	"context"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Value of _enforcePercentage doesn't matter, we just confirm output matches
// input.
const _enforcePercentage = 0.42

func TestInboundReport(t *testing.T) {
	var testVars = []struct {
		descr      string
		claim      *wonka.Claim
		err        internal.InboundAuthenticationError
		enforced   bool
		derelict   bool
		zapLevel   zapcore.Level
		zapMessage string
		zapFields  []zapcore.Field
		spanFields []mocktracer.MockLogRecord
		m3tags     map[string]string
	}{
		{
			descr:      "denied",
			err:        internal.ErrNoToken,
			enforced:   true,
			zapLevel:   zap.InfoLevel,
			zapMessage: "denying unauthenticated request",
			zapFields: []zapcore.Field{
				zap.Skip(), // destination
				zap.Skip(), // remote_entity
				zap.Bool("has_baggage", false),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "denied"),
				zap.String("unauthorized_reason", "no_token"),
			},
			spanFields: testhelper.ExpectedInboundSpanFields(false, _enforcePercentage, 0, "", ""),
			m3tags: map[string]string{
				"allowed":             "denied",
				"has_baggage":         "false",
				"is_derelict":         "false",
				"unauthorized_reason": "no_token",
			},
		},
		{
			descr:      "denied with malformed token in baggage",
			err:        internal.NewMalformedTokenError("error unmarshalling token"),
			enforced:   true,
			zapLevel:   zap.InfoLevel,
			zapMessage: "denying unauthenticated request",
			zapFields: []zapcore.Field{
				zap.Skip(), // destination
				zap.Skip(), // remote_entity
				zap.Bool("has_baggage", true),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "denied"),
				zap.String("unauthorized_reason", "malformed_token"),
			},
			spanFields: testhelper.ExpectedInboundSpanFields(true, _enforcePercentage, 0, "", ""),
			m3tags: map[string]string{
				"allowed":             "denied",
				"has_baggage":         "true",
				"is_derelict":         "false",
				"unauthorized_reason": "malformed_token",
			},
		},
		{
			descr:      "denied with invalid claim",
			claim:      &wonka.Claim{Destination: "my-service", EntityName: "some-other-service"},
			err:        internal.NewInvalidTokenError("validation fail"),
			enforced:   true,
			zapLevel:   zap.InfoLevel,
			zapMessage: "denying unauthenticated request",
			zapFields: []zapcore.Field{
				zap.String("destination", "my-service"),
				zap.String("remote_entity", "some-other-service"),
				zap.Bool("has_baggage", true),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "denied"),
				zap.String("unauthorized_reason", "invalid_token"),
			},
			spanFields: testhelper.ExpectedInboundSpanFields(true, _enforcePercentage, 0, "my-service", "some-other-service"),
			m3tags: map[string]string{
				"allowed":             "denied",
				"has_baggage":         "true",
				"is_derelict":         "false",
				"remote_entity":       "some-other-service",
				"unauthorized_reason": "invalid_token",
			},
		},
		{
			descr: "not enforced",
			claim: &wonka.Claim{Destination: "my-service", EntityName: "some-other-service"},
			err: internal.NewInboundAuthenticationErrorf(
				internal.UnauthorizedNoCommonClaims, true, "no common claims",
			),
			enforced:   false,
			zapLevel:   zap.InfoLevel,
			zapMessage: "allowing unauthenticated request",
			zapFields: []zapcore.Field{
				zap.String("destination", "my-service"),
				zap.String("remote_entity", "some-other-service"),
				zap.Bool("has_baggage", true),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "not_enforced"),
				zap.String("unauthorized_reason", "no_common_claims"),
			},
			spanFields: testhelper.ExpectedInboundSpanFields(true, _enforcePercentage, 1, "my-service", "some-other-service"),
			m3tags: map[string]string{
				"allowed":             "not_enforced",
				"has_baggage":         "true",
				"is_derelict":         "false",
				"remote_entity":       "some-other-service",
				"unauthorized_reason": "no_common_claims",
			},
		},
		{
			descr:      "derelict",
			err:        internal.ErrNoToken,
			enforced:   true,
			derelict:   true,
			zapLevel:   zap.InfoLevel,
			zapMessage: "allowing unauthenticated request",
			zapFields: []zapcore.Field{
				zap.Skip(), // destination
				zap.Skip(), // remote_entity
				zap.Bool("has_baggage", false),
				zap.Bool("is_derelict", true),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "not_enforced"),
				zap.String("unauthorized_reason", "no_token"),
			},
			spanFields: testhelper.ExpectedInboundSpanFields(false, _enforcePercentage, 1, "", ""),
			m3tags: map[string]string{
				"allowed":             "not_enforced",
				"has_baggage":         "false",
				"is_derelict":         "true",
				"unauthorized_reason": "no_token",
			},
		},
		{
			descr:      "allowed",
			claim:      &wonka.Claim{Destination: "my-service", EntityName: "some-other-service"},
			zapLevel:   zap.DebugLevel,
			zapMessage: "request successfully authenticated",
			zapFields: []zapcore.Field{
				zap.String("destination", "my-service"),
				zap.String("remote_entity", "some-other-service"),
				zap.Bool("has_baggage", true),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "allowed"),
				zap.String("unauthorized_reason", ""), // no reason
			},
			spanFields: testhelper.ExpectedInboundSpanFields(true, _enforcePercentage, 2, "my-service", "some-other-service"),
			m3tags: map[string]string{
				"allowed":       "allowed",
				"has_baggage":   "true",
				"is_derelict":   "false",
				"remote_entity": "some-other-service",
			},
		},
		{
			descr:      "allowed personnel entity",
			claim:      &wonka.Claim{Destination: "my-service", EntityName: "engineerjoe@example.com"},
			zapLevel:   zap.DebugLevel,
			zapMessage: "request successfully authenticated",
			zapFields: []zapcore.Field{
				zap.String("destination", "my-service"),
				zap.String("remote_entity", "engineerjoe@example.com"),
				zap.Bool("has_baggage", true),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "allowed"),
				zap.String("unauthorized_reason", ""), // no reason
			},
			spanFields: testhelper.ExpectedInboundSpanFields(true, _enforcePercentage, 2, "my-service", "engineerjoe@example.com"),
			m3tags: map[string]string{
				"allowed":       "allowed",
				"has_baggage":   "true",
				"is_derelict":   "false",
				"remote_entity": "personnel_entity_redacted",
			},
		},
		{
			descr: "allowed without claim or baggage",
			err:   nil, // AuthenticateIn should never actually use this case because having
			// no claim is an error condition.
			enforced:   true,
			zapLevel:   zap.DebugLevel,
			zapMessage: "request successfully authenticated",
			zapFields: []zapcore.Field{
				zap.Skip(), // destination
				zap.Skip(), // remote_entity
				zap.Bool("has_baggage", false),
				zap.Bool("is_derelict", false),
				zap.Float64("enforce_percentage", _enforcePercentage),
				zap.String("allowed", "allowed"),
				zap.String("unauthorized_reason", ""), // no reason
			},
			spanFields: testhelper.ExpectedInboundSpanFields(false, _enforcePercentage, 2, "", ""),
			m3tags: map[string]string{
				"allowed":     "allowed",
				"has_baggage": "false",
				"is_derelict": "false",
			},
		},
	}

	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			// Set up mock metric targets we can interogate.
			metrics := tally.NewTestScope("", map[string]string{})
			_, ctx, span := testhelper.SetupContext()
			obs, logs := observer.New(zap.DebugLevel)
			logger := zap.New(obs)

			// System under test

			reporter := telemetry.NewInboundReporter(logger, metrics, _enforcePercentage)
			if m.claim != nil {
				reporter.SetClaim(m.claim)
			}
			reporter.Report(ctx, m.err, m.enforced, m.derelict)

			// Assertions

			if len(m.zapFields) > 0 {
				// ctx isn't defined until within Run, so we could not add the
				// Trace field when we defined testVars.
				m.zapFields = append([]zapcore.Field{zap.Error(m.err)}, m.zapFields...)
				m.zapFields = append(m.zapFields, jaegerzap.Trace(ctx))
			}
			testhelper.AssertOneZapLog(t, logs, m.zapLevel, m.zapMessage, m.zapFields)

			testhelper.AssertSpanFieldsLogged(t, span, m.spanFields)

			if len(m.m3tags) == 0 {
				testhelper.AssertNoM3Counters(t, metrics)
			} else {
				testhelper.AssertOneM3Counter(t, metrics, "in", 1, m.m3tags)
			}
		})
	}
}

// TestInboundReportNoSpan covers the degenerate case context is nil.
// uberGalileo would have probably died before calling Report.
func TestInboundReportNilContext(t *testing.T) {
	metrics := tally.NewTestScope("", map[string]string{})
	obs, logs := observer.New(zap.DebugLevel)
	logger := zap.New(obs)

	var ctx context.Context // nil context without annoying linter SA1012

	reporter := telemetry.NewInboundReporter(logger, metrics, _enforcePercentage)
	reporter.Report(ctx, internal.ErrNoSpan, true, false)

	testhelper.AssertOneZapLog(t, logs, zapcore.InfoLevel,
		"denying unauthenticated request",
		[]zapcore.Field{
			zap.Error(internal.ErrNoSpan),
			zap.Skip(), // destination
			zap.Skip(), // remote_entity
			zap.Bool("has_baggage", false),
			zap.Bool("is_derelict", false),
			zap.Float64("enforce_percentage", _enforcePercentage),
			zap.String("allowed", "denied"),
			zap.String("unauthorized_reason", "no_token"),
			jaegerzap.Trace(ctx),
		})

	testhelper.AssertOneM3Counter(t, metrics, "in", 1,
		map[string]string{
			"allowed":             "denied",
			"has_baggage":         "false",
			"is_derelict":         "false",
			"unauthorized_reason": "no_token",
		},
	)
}

// TestInboundReportNoSpan covers the degenerate case where the context has no
// span. uberGalileo would have added a span before calling Report.
func TestInboundReportNoSpan(t *testing.T) {
	metrics := tally.NewTestScope("", map[string]string{})
	obs, logs := observer.New(zap.DebugLevel)
	logger := zap.New(obs)

	ctx := context.Background()

	reporter := telemetry.NewInboundReporter(logger, metrics, _enforcePercentage)
	reporter.Report(ctx, internal.ErrNoSpan, true, false)

	testhelper.AssertOneZapLog(t, logs, zapcore.InfoLevel,
		"denying unauthenticated request",
		[]zapcore.Field{
			zap.Error(internal.ErrNoSpan),
			zap.Skip(), // destination
			zap.Skip(), // remote_entity
			zap.Bool("has_baggage", false),
			zap.Bool("is_derelict", false),
			zap.Float64("enforce_percentage", _enforcePercentage),
			zap.String("allowed", "denied"),
			zap.String("unauthorized_reason", "no_token"),
			jaegerzap.Trace(ctx),
		})

	testhelper.AssertOneM3Counter(t, metrics, "in", 1,
		map[string]string{
			"allowed":             "denied",
			"has_baggage":         "false",
			"is_derelict":         "false",
			"unauthorized_reason": "no_token",
		},
	)
}
