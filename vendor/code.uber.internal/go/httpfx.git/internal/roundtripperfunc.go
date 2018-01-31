package internal

import "net/http"

// RoundTripperFunc is an http.RoundTripper based on a function.
type RoundTripperFunc func(r *http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (t RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return t(r)
}
