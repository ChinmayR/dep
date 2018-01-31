package internal

import "fmt"

// UnauthorizedReason represents the reason an inbound request will not be StatusAllowedAllOK
type UnauthorizedReason string

// Values for UnauthorizedReason defined for cross-language consistency in
// https://docs.google.com/document/d/1uG6EwaBxBJRCz-sJVbiNo716zYgqhDZigX-BZdjtKAI/
const (
	// UnauthorizedNoReason is the default value before making and authentication
	// decision.
	//UnauthorizedNoReason UnauthorizedReason = "no_reason_given"
	// UnauthorizedNoToken means wonka authentication was missing from Jaeger baggage.
	UnauthorizedNoToken UnauthorizedReason = "no_token"
	// UnauthorizedMalformedToken means the given token could not be
	// unmarshalled, e.g. invalid json.
	UnauthorizedMalformedToken UnauthorizedReason = "malformed_token"
	// UnauthorizedInvalidToken means the given token was well formed and failed
	// validation, e.g. bad signature or validity period.
	UnauthorizedInvalidToken UnauthorizedReason = "invalid_token"
	// UnauthorizedNoCommonClaims means none of the claims required by the server's
	// Galileo configuration are included in the given wonka token.
	UnauthorizedNoCommonClaims UnauthorizedReason = "no_common_claims"
	// UnauthorizedRemoteEntityMismatch means wonka token affirmed an entity name
	// different from X-Uber-Source
	UnauthorizedRemoteEntityMismatch UnauthorizedReason = "remote_entity_mismatch"
	// UnauthorizedWrongDestination means the destination in the wonka token is not
	// allowed by the server's Galileo configuration.
	UnauthorizedWrongDestination UnauthorizedReason = "wrong_destination"
)

func (r UnauthorizedReason) String() string {
	return string(r)
}

// InboundAuthenticationError occurs when an inbound request context does not
// contain a valid authentication token.
type InboundAuthenticationError interface {
	// InboundAuthenticationError is a custom error type
	error

	// HasBaggage returns true if error condition occurred after finding auth
	// baggage.
	HasBaggage() bool

	// Reason returns why the request is not authorized as a low cardinality enum,
	// suitable for M3 metrics.
	Reason() UnauthorizedReason
}

type inboundAuthenticationError struct {
	msg        string
	hasBaggage bool
	reason     UnauthorizedReason
}

// NewInboundAuthenticationErrorf is the general constructor.
func NewInboundAuthenticationErrorf(reason UnauthorizedReason, hasBaggage bool, format string, a ...interface{}) InboundAuthenticationError {
	return inboundAuthenticationError{
		msg:        fmt.Sprintf(format, a...),
		hasBaggage: hasBaggage,
		reason:     reason,
	}
}

// NewInvalidTokenError occurs when Wonka token is well formed by does not pass
// validation, e.g. expired or bad signature.
func NewInvalidTokenError(msg string) InboundAuthenticationError {
	return inboundAuthenticationError{
		msg:        msg,
		hasBaggage: true,
		reason:     UnauthorizedInvalidToken,
	}
}

// NewMalformedTokenError occurs when Wonka token cannot be unmarshalled.
func NewMalformedTokenError(msg string) InboundAuthenticationError {
	return inboundAuthenticationError{
		msg:        msg,
		hasBaggage: true,
		reason:     UnauthorizedMalformedToken,
	}
}

// Error implements InboundAuthenticationError. Returns human readable string.
func (err inboundAuthenticationError) Error() string {
	return err.msg
}

// Error implements InboundAuthenticationError.
func (err inboundAuthenticationError) HasBaggage() bool {
	return err.hasBaggage
}

// Error implements InboundAuthenticationError.
func (err inboundAuthenticationError) Reason() UnauthorizedReason {
	return err.reason
}

var (
	// ErrNoSpan occurs when incoming context has no Jaeger span.
	ErrNoSpan = inboundAuthenticationError{
		msg:        "cannot retrieve baggage, context has no span. Integrate Jaeger https://engdocs.uberinternal.com/jaeger/",
		hasBaggage: false,
		reason:     UnauthorizedNoToken,
	}

	// ErrNoToken occurs when incoming context doesn't auth baggage.
	ErrNoToken = inboundAuthenticationError{
		msg:        "unauthenticated request: no wonka token in baggage",
		hasBaggage: false,
		reason:     UnauthorizedNoToken,
	}
)
