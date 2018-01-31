package telemetry

import (
	"context"
	"strconv"

	"code.uber.internal/engsec/galileo-go.git/internal"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InboundRequestStatus encapsulates the possible ways AuthenticateIn will handle the
// incoming request.
type InboundRequestStatus struct {
	i int
	s string
}

// Not using iota because these integer values end up in Jaeger logs.
// String values end up in M3 and ELK.
// Both and are defined for cross-language consistency in
// https://docs.google.com/document/d/1hmlcR2Fqx_jtNMTIeOyi9xPx4sODOukXAdVbTdP1u5I/
var (
	// NoStatusYet means AuthenticateIn hasn't reported a status yet.
	NoStatusYet = InboundRequestStatus{}
	// StatusDenied means service is enforcing authentication and request was
	// rejected because either auth baggage is not present in the request, or
	// there are other issues with the Wonka token.
	StatusDenied = InboundRequestStatus{0, "denied"}
	// StatusNotEnforced means there are issues with the Wonka token, i.e. the
	// request is not authenticated, but the request is being let through due to
	// enforce_percentage being set less than 1.
	StatusNotEnforced = InboundRequestStatus{1, "not_enforced"}
	// StatusAllowedAllOK means the request is properly authenticated, i.e.
	// everything is good with the Wonka token.
	StatusAllowedAllOK = InboundRequestStatus{2, "allowed"}
)

// LogMessage converts to an easily human readable form to be used in logs.
func (r InboundRequestStatus) LogMessage() string {
	switch r {
	case StatusDenied:
		return "denying unauthenticated request"
	case StatusNotEnforced:
		return "allowing unauthenticated request"
	case StatusAllowedAllOK:
		return "request successfully authenticated"
	default:
		return "unknown authentication status"
	}
}

// Int returns an integer representation for use decorating Jaeger spans.
func (r InboundRequestStatus) Int() int {
	return r.i
}

// String returns a string representation for use in logs and metrics.
func (r InboundRequestStatus) String() string {
	return r.s
}

// InboundReporter is responsible for keeping state of an AuthenticateIn call and
// emitting the proper metrics and logs about the result.
type InboundReporter struct {
	log                *zap.Logger
	metrics            tally.Scope
	err                error
	enforcePercentage  float64
	isDerelict         bool
	hasBaggage         bool
	unauthorizedReason internal.UnauthorizedReason
	status             InboundRequestStatus
	claim              *wonka.Claim
}

// NewInboundReporter constructs an InboundReporter with initial default values.
func NewInboundReporter(log *zap.Logger, metrics tally.Scope, enforcePercentage float64) *InboundReporter {
	return &InboundReporter{
		log:               log,
		metrics:           metrics,
		enforcePercentage: enforcePercentage,
	}
}

// SetClaim indicates the inbound request had a valid wonka claim.
func (itr *InboundReporter) SetClaim(claim *wonka.Claim) {
	itr.claim = claim
	itr.hasBaggage = true
}

// Report emits appropriate logs and metrics, and decorates the span inside the
// given context, using current state of the telemetry as well as the given
// error and enforcement decision.
func (itr *InboundReporter) Report(ctx context.Context, err internal.InboundAuthenticationError, enforced, derelict bool) {
	itr.err = err
	itr.isDerelict = derelict

	if err == nil {
		itr.status = StatusAllowedAllOK
	} else {
		itr.hasBaggage = err.HasBaggage()
		itr.unauthorizedReason = err.Reason()

		itr.status = StatusDenied
		if !enforced || derelict {
			itr.status = StatusNotEnforced
		}
	}

	itr.metrics.Tagged(itr.m3Tags()).Counter("in").Inc(1)
	itr.reportToLog(ctx)
	itr.reportToSpan(ctx)
}

// m3Tags returns tags so metrics can be analyzed using M3.
func (itr *InboundReporter) m3Tags() map[string]string {
	m3tags := map[string]string{
		"has_baggage": strconv.FormatBool(itr.hasBaggage),
		"is_derelict": strconv.FormatBool(itr.isDerelict),
		"allowed":     itr.status.String(),
	}
	if itr.status != StatusAllowedAllOK {
		m3tags["unauthorized_reason"] = itr.unauthorizedReason.String()
	}
	if itr.claim != nil {
		m3tags["remote_entity"] = SanitizeEntityName(itr.claim.EntityName)
	}
	return m3tags
}

// reportToLog writes an annotated log message.
func (itr *InboundReporter) reportToLog(ctx context.Context) {
	logger := itr.log.Debug
	if itr.status != StatusAllowedAllOK {
		logger = itr.log.Info
	}

	logger(itr.status.LogMessage(),
		zap.Error(itr.err), // zap skips this field when err is nil
		itr.destinationZapField(),
		itr.remoteEntityZapField(),
		zap.Bool("has_baggage", itr.hasBaggage),
		zap.Bool("is_derelict", itr.isDerelict),
		zap.Float64("enforce_percentage", itr.enforcePercentage),
		// Using String instead of Stringer allows me to write tests independent
		// of implementation details like which types are used to represent
		// status and reason.
		zap.String("allowed", itr.status.String()),
		zap.String("unauthorized_reason", itr.unauthorizedReason.String()),
		jaegerzap.Trace(ctx), // handles nil context and nil span
	)
}

// destinationZapField returns the appropriate zap log annotation depending on if
// there is a claim with a destination.
func (itr *InboundReporter) destinationZapField() zapcore.Field {
	if itr.claim == nil {
		return zap.Skip()
	}
	return zap.String("destination", itr.claim.Destination)
}

// remoteEntityZapField  returns the appropriate zap log annotation depending on if
// there is a claim with an entity name.
func (itr *InboundReporter) remoteEntityZapField() zapcore.Field {
	if itr.claim == nil {
		return zap.Skip()
	}
	return zap.String("remote_entity", itr.claim.EntityName)
}

// reportToSpan adds log fields to span in the given context so results are
// visible in Jaeger.
func (itr *InboundReporter) reportToSpan(ctx context.Context) {
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
		otlog.Bool("galileo.in.has_baggage", itr.hasBaggage),
		otlog.String("galileo.in.version", internal.LibraryVersion()),
		otlog.Float64("galileo.in.enforce_percentage", itr.enforcePercentage),
		otlog.Int("galileo.in.allowed", itr.status.Int()),
	)
	if itr.claim != nil {
		span.LogFields(
			otlog.String("galileo.in.destination", itr.claim.Destination),
			otlog.String("galileo.in.entity_name", itr.claim.EntityName),
		)
	}
}
