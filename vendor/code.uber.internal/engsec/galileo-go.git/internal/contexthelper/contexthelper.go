package contexthelper

import (
	"context"

	"code.uber.internal/engsec/galileo-go.git/internal"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/opentracing/opentracing-go"
)

// ServiceAuthBaggageAttr is the key where wonka claim is stored in jaeger baggage.
const ServiceAuthBaggageAttr = "x-wonka-auth"

// _galileoSpanName is the name for Jaeger spans started by Galileo library in
// the case when no span was started previously.
const _galileoSpanName = "galileo"

// EnsureSpan creates a new context with a new span if the given context has no
// span. When a span is created the returned finish function will finish that
// span. When a span already exists, finish is a noop.
func EnsureSpan(ctx context.Context, tracer opentracing.Tracer) (_ context.Context, finish func()) {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		// Everything is fine, nothing for us to do.
		return ctx, func() {}
	}
	return AddSpan(ctx, tracer)
}

// AddSpan returns a new context with a newly created span and a function to
// finish that span.
func AddSpan(ctx context.Context, tracer opentracing.Tracer) (_ context.Context, finish func()) {
	var parentSpanContext opentracing.SpanContext
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		parentSpanContext = parentSpan.Context()
	}
	span := tracer.StartSpan(_galileoSpanName, opentracing.ChildOf(parentSpanContext))
	return opentracing.ContextWithSpan(ctx, span), span.Finish
}

// SetBaggage modifies span by adding claimToken to Jaeger baggage using the
// standard key.
// If a claim token has already been added, it will be overwritten.
func SetBaggage(span opentracing.Span, claimToken string) {
	span.SetBaggageItem(ServiceAuthBaggageAttr, claimToken)
}

// ClaimFromContext returns the claim from Jaeger baggage or values describing
// why that cannot be done. It is meant to operate on incoming context.
// The returned UnauthorizedReason is machine readable, error is human readable.
// An empty string is returned if there's no claim.
// An error is returned if there is no span.
func ClaimFromContext(ctx context.Context) (*wonka.Claim, internal.InboundAuthenticationError) {
	tokenStr, err := StripTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return UnmarshalToken(tokenStr)
}

// StripTokenFromContext retrieves the marshalled token from the context, and
// then removes it so that it doesn't propagate further.
func StripTokenFromContext(ctx context.Context) (string, internal.InboundAuthenticationError) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		// Indicates service owner has not onboarded to Jaeger properly.
		return "", internal.ErrNoSpan
	}

	tokenStr := span.BaggageItem(ServiceAuthBaggageAttr)
	if tokenStr == "" {
		return "", internal.ErrNoToken
	}

	// Token is valid for only a single hop. Prevent it from leaking outside
	// Uber or to other Uber services in the remote procedure call chain.
	SetBaggage(span, "")

	return tokenStr, nil
}

// UnmarshalToken takes a claim token string and converts it to a wonka Claim object.
func UnmarshalToken(tokenStr string) (*wonka.Claim, internal.InboundAuthenticationError) {
	claim, err := wonka.UnmarshalClaim(tokenStr)
	if err != nil {
		return nil, internal.NewMalformedTokenError(err.Error())
	}
	return claim, nil
}
