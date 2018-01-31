package galileohttp

import (
	"net/http"

	galileo "code.uber.internal/engsec/galileo-go.git"
)

// RPCServiceHeader is the name of the HTTP request header under which the
// name of the destination service is stored.
const RPCServiceHeader = "Rpc-Service"

// AuthenticateOutMiddleware builds a middleware that authenticates all
// outgoing requests with the given Galileo client. The middleware expects the
// RoundTripper to have Jaeger tracing support.
//
// If a request doesn't have a destination service name associated with it,
// the request will be sent without any authentication. The destination
// service name for a request may be provided by specifying it in the
// Rpc-Service header.
func AuthenticateOutMiddleware(g galileo.Galileo) func(http.RoundTripper) http.RoundTripper {
	// TODO(abg): Other means to provide destination services. Preferably, the
	// go/http library will provide a way to set the destination service name
	// and we'll read it here.

	return func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			rt = http.DefaultTransport
		}
		return authTransport{g: g, transport: rt}
	}
}

type authTransport struct {
	g         galileo.Galileo
	transport http.RoundTripper
}

var _ http.RoundTripper = authTransport{}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if dest := req.Header.Get(RPCServiceHeader); dest != "" {
		// Authenticate outgoing requests only if the destination service is
		// set.
		ctx, err := t.g.AuthenticateOut(req.Context(), dest)
		if err != nil {
			return nil, err
		}
		req = req.WithContext(ctx)
	}

	return t.transport.RoundTrip(req)
}
