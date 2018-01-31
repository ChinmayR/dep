package telemetry

import (
	"context"
	"strconv"

	"code.uber.internal/engsec/galileo-go.git/internal"

	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
)

// OutboundReporter is responsible for keeping state of an AuthenticateIn call and
// emitting the proper metrics and logs about the result.
type OutboundReporter struct {
	log           *zap.Logger
	metrics       tally.Scope
	destination   string
	entityName    string // name of local entity
	explicitClaim string
	hasBaggage    bool
}

// NewOutboundReporter constructs an OutboundReporter with initial default
// values.
func NewOutboundReporter(log *zap.Logger, metrics tally.Scope, destination, explicitClaim, entityName string) *OutboundReporter {
	return &OutboundReporter{
		log:           log,
		metrics:       metrics,
		destination:   destination,
		entityName:    entityName,
		explicitClaim: explicitClaim,
	}
}

// Report emits appropriate logs and metrics, and decorates the span inside the
// given context, with the current state of the telemetry reporter.
func (otr *OutboundReporter) Report(ctx context.Context, err error) {
	otr.metrics.Tagged(map[string]string{
		"has_baggage": strconv.FormatBool(otr.hasBaggage),
		"destination": otr.destination,
	}).Counter("out").Inc(1)
	otr.reportToLog(ctx, err)
	otr.reportToSpan(ctx)
}

// SetHasBaggage indicates whether or not wonka auth baggage has been added to
// the outbound request context.
func (otr *OutboundReporter) SetHasBaggage(b bool) {
	otr.hasBaggage = b
}

// reportToLog writes an annotated log message to the given logger.
func (otr *OutboundReporter) reportToLog(ctx context.Context, err error) {
	logger := otr.log.Debug
	msg := "authenticate out succeeded"
	if err != nil {
		logger = otr.log.Warn
		msg = "authenticate out failed"
	}

	// "entity":entityName already added to log by Galileo.
	logger(msg,
		zap.Error(err), // zap skips this field when err is nil
		zap.Bool("has_baggage", otr.hasBaggage),
		zap.String("destination", otr.destination),
		zap.String("claim", otr.explicitClaim),
		jaegerzap.Trace(ctx), // handles nil ctx and nil span
	)
}

// reportToSpan adds log fields to span in the given context so results are
// visible in Jaeger.
func (otr *OutboundReporter) reportToSpan(ctx context.Context) {
	if ctx == nil {
		// This is highly unexpected, however having no span logs is better than
		// SpanFromContext throwing a nil pointer exception.
		return
	}
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return
	}
	span.LogFields(
		otlog.Bool("galileo.out.has_baggage", otr.hasBaggage),
		otlog.String("galileo.out.version", internal.LibraryVersion()),
		otlog.String("galileo.out.destination", otr.destination),
		otlog.String("galileo.out.entity_name", otr.entityName),
	)
}
