package internal

import (
	"context"
	"errors"
	"fmt"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// serviceAuthBaggageAttr is the key where wonka claim is stored in jaeger baggage.
const serviceAuthBaggageAttr = "x-wonka-auth"

// galileoSpanName is the name for Jaeger spans started by Galileo library in
// the case when no span was started previously.
const galileoSpanName = "galileo"

var (
	// errorSpanlessGet indicates a claim cannot be retrieved from baggage
	// because the context has no Jaeger span.
	errorSpanlessGet = errors.New("galileo: Cannot retrieve baggage, context has no span. Integrate Jaeger https://engdocs.uberinternal.com/jaeger/")

	// errorSpanlessSet indicates a claim cannot be added to baggage
	// because the context has no Jaeger span.
	errorSpanlessSet = errors.New("galileo: Cannot set baggage, context has no span. Integrate Jaeger https://engdocs.uberinternal.com/jaeger/")
)

const (
	tagOutHasBaggage  = "galileo.out.has_baggage" // does the outbound request contain auth baggage
	tagOutDestination = "galileo.out.destination" // aka callee
	tagOutEntityName  = "galileo.out.entity_name" // aka caller, making and tagging outbound request
	tagOutVersion     = "galileo.out.version"

	tagInHasBaggage = "galileo.in.has_baggage" // does the inbound request contain auth baggage
	tagInVersion    = "galileo.in.version"

	// These tag names are public because we cannot learn the proper value
	// inside this file, they must be set elsewhere.
	TagInAllowed        = "galileo.in.allowed"
	TagInDestination    = "galileo.in.destination" // should be the callee receiving and tagging inbound request. Unless we received a claim destined for someone else.
	TagInEnforcePercent = "galileo.in.enforce_percentage"
	TagInEntityName     = "galileo.in.entity_name" // aka caller

	// Possible values for TagInAllowed
	AllowedAllOK = 2
	NotEnforced  = 1
	Denied       = 0
)

// Log the language and version so we can track adoption.
// Differentiates us from the galileo libraries in java, python, node, etc.
func libraryVersion() string {
	return fmt.Sprintf("galileo-go: %s", Version)
}

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
	span := tracer.StartSpan(galileoSpanName, opentracing.ChildOf(parentSpanContext))
	return opentracing.ContextWithSpan(ctx, span), span.Finish
}

// SetBaggage modifies ctx by adding claim to Jaeger baggage, and
// tagging the current span to allow analytics.
// If a claim value has already been set, it will be overwritten.
func SetBaggage(ctx context.Context, name, destination, claim string) error {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		// Indicates service owner has not onboarded to Jaeger properly.
		return errorSpanlessSet
	}

	span.SetBaggageItem(serviceAuthBaggageAttr, claim)

	span.LogFields(
		log.Bool(tagOutHasBaggage, true),
		log.String(tagOutDestination, destination),
		log.String(tagOutEntityName, name),
		log.String(tagOutVersion, libraryVersion()),
	)
	return nil
}

// ClaimFromContext sets tags on the  span, and returns the claim from Jaeger
// baggage. It is meant to operate on incoming context.
// An empty string is returned if there's no claim.
// An error is returned if there is no span.
func ClaimFromContext(ctx context.Context) (string, error) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		// Indicates service owner has not onboarded to Jaeger properly.
		return "", errorSpanlessGet
	}

	claim := span.BaggageItem(serviceAuthBaggageAttr)

	span.LogFields(
		log.Bool(tagInHasBaggage, claim != ""),
		log.String(tagInVersion, libraryVersion()),
	)

	return claim, nil
}
