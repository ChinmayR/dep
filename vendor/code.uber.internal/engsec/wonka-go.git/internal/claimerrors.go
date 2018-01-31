package internal

import "fmt"

// This cannot go in claimhelper because it would form a circular dependency.

// InvalidityReason represents a reason a wonka.Claim is not valid for a
// specific purpose.
type InvalidityReason string

// Values for InvalidityReason defined for cross-language consistency in
// https://docs.google.com/document/d/1uG6EwaBxBJRCz-sJVbiNo716zYgqhDZigX-BZdjtKAI/
const (
	// InvalidFromFuture means token is not valid because c.ValidAfter is in the
	// future. Does not guarantee token will be valid.
	InvalidFromFuture InvalidityReason = "future_token"

	// InvalidExpired means token is not valid because c.ValidBefore is in the
	// past. Does not guarantee token was ever valid.
	InvalidExpired InvalidityReason = "expired_token"

	// InvalidMarshalError means token could not be marshalled for signature
	// checking.
	InvalidMarshalError InvalidityReason = "marshalling_error"

	// InvalidSignature means signature did not match token content, or was
	// signed by an unkown wonkamaster key.
	InvalidSignature InvalidityReason = "invalid_signature"

	// InvalidNoCommonClaims means token doesn't include any claims required in
	// a call to Claim.Check or Claim.Inspect.
	InvalidNoCommonClaims InvalidityReason = "no_common_claims"

	// InvalidWrongDestination means token destination is not amoung the
	// destinations required in a call to Claim.Check or Claim.Inspect.
	InvalidWrongDestination InvalidityReason = "wrong_destination"
)

// Ensure ValidationError also implements the error interface.
var _ error = (*ValidationError)(nil)

// ValidationError includes machine readable information about errors
// returned by wonka.Claim.{Validate,Check,Inspect}.
type ValidationError struct {
	msg    string
	reason InvalidityReason
}

// NewValidationErrorf returns a new ValidationError with the given reason and
// an error message formatted using Sprintf.
func NewValidationErrorf(reason InvalidityReason, format string, a ...interface{}) *ValidationError {
	return &ValidationError{
		msg:    fmt.Sprintf(format, a...),
		reason: reason,
	}
}

// Error implements the error interface
func (err *ValidationError) Error() string {
	return err.msg
}

// Reason returns the low cardinality machine readable validation problem.
func (err *ValidationError) Reason() string {
	// Cast to string so we don't expose internal types.
	return string(err.reason)
}
