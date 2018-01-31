package telemetry_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestOutboundReport(t *testing.T) {
	var testVars = []struct {
		descr      string
		hasBaggage bool
		err        error
	}{
		{
			descr:      "yes baggage",
			hasBaggage: true,
		},
		{
			descr:      "no baggage",
			hasBaggage: false,
		},
		{
			descr:      "error",
			hasBaggage: true,
			err:        errors.New("something went wrong"),
		},
	}

	for _, m := range testVars {
		t.Run(m.descr, func(t *testing.T) {
			// Set up mock metric targets we can interogate.
			metrics := tally.NewTestScope("", map[string]string{})
			_, ctx, span := testhelper.SetupContext()
			obs, logs := observer.New(zap.DebugLevel)
			logger := zap.New(obs)

			destination := "over-yonder"
			explicitClaim := "some-specific-claim"
			entityName := "some-entity"

			// System under test

			reporter := telemetry.NewOutboundReporter(logger, metrics, destination, explicitClaim, entityName)
			reporter.SetHasBaggage(m.hasBaggage)
			reporter.Report(ctx, m.err)

			// Assertions

			expectedZapLevel := zapcore.DebugLevel
			expectedZapMessage := "authenticate out succeeded"
			if m.err != nil {
				expectedZapLevel = zapcore.WarnLevel
				expectedZapMessage = "authenticate out failed"
			}

			testhelper.AssertOneZapLog(t, logs, expectedZapLevel, expectedZapMessage,
				[]zapcore.Field{
					zap.Error(m.err),
					zap.Bool("has_baggage", m.hasBaggage),
					zap.String("destination", destination),
					zap.String("claim", explicitClaim),
					jaegerzap.Trace(ctx),
				})

			testhelper.AssertSpanFieldsLogged(
				t, span,
				testhelper.ExpectedOutboundSpanFields(m.hasBaggage, destination, entityName),
			)

			testhelper.AssertOneM3Counter(t, metrics, "out", 1,
				map[string]string{
					"has_baggage": strconv.FormatBool(m.hasBaggage),
					"destination": destination,
				},
			)
		})
	}
}

// TestOutboundReportNoSpan covers the degenerate case when context is nil.
// uberGalileo would have probably died before calling Report.
func TestOutboundReportNilContext(t *testing.T) {
	metrics := tally.NewTestScope("", map[string]string{})
	obs, logs := observer.New(zap.DebugLevel)
	logger := zap.New(obs)

	destination := "over-yonder"
	explicitClaim := "some-specific-claim"
	entityName := "some-entity"
	var ctx context.Context // nil context without annoying linter SA1012
	err := errors.New("everything has all gone wrong")

	// System under test

	reporter := telemetry.NewOutboundReporter(logger, metrics, destination, explicitClaim, entityName)
	reporter.Report(ctx, err)

	// Assertions

	testhelper.AssertOneZapLog(t, logs, zapcore.WarnLevel, "authenticate out failed",
		[]zapcore.Field{
			zap.Error(err),
			zap.Bool("has_baggage", false),
			zap.String("destination", destination),
			zap.String("claim", explicitClaim),
			jaegerzap.Trace(ctx),
		})

	testhelper.AssertOneM3Counter(t, metrics, "out", 1,
		map[string]string{
			"has_baggage": "false",
			"destination": destination,
		},
	)
}

// TestOutboundReportNoSpan covers the degenerate case where the context has no
// span. uberGalileo would have added a span before calling Report.
func TestOutboundReportNoSpan(t *testing.T) {
	metrics := tally.NewTestScope("", map[string]string{})
	obs, logs := observer.New(zap.DebugLevel)
	logger := zap.New(obs)

	destination := "over-yonder"
	explicitClaim := "some-specific-claim"
	entityName := "some-entity"
	err := errors.New("everything has all gone wrong")
	ctx := context.Background()

	// System under test

	reporter := telemetry.NewOutboundReporter(logger, metrics, destination, explicitClaim, entityName)
	reporter.Report(ctx, err)

	// Assertions

	testhelper.AssertOneZapLog(t, logs, zapcore.WarnLevel, "authenticate out failed",
		[]zapcore.Field{
			zap.Error(err),
			zap.Bool("has_baggage", false),
			zap.String("destination", destination),
			zap.String("claim", explicitClaim),
			jaegerzap.Trace(ctx),
		})

	testhelper.AssertOneM3Counter(t, metrics, "out", 1,
		map[string]string{
			"has_baggage": "false",
			"destination": destination,
		},
	)
}
