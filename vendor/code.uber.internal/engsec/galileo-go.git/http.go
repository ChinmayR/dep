package galileo

import (
	"context"
	"net/http"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"go.uber.org/zap"
)

// AuthenticateHTTPRequest authenticates a request to a particular HTTP
// endpoint.
//
// It uses the endpoint-specific configuration for the given request, or the
// global configuration if no endpoint-specific configuration is found.
//
// Compares Rpc-Caller and X-Uber-Source headers against derelict services list.
// Value of Rpc-Caller is used when both headers are present.
func AuthenticateHTTPRequest(ctx context.Context, r *http.Request, g Galileo) error {
	var allowedEntities []string
	if ecfg, err := g.Endpoint(r.URL.Path); err == nil {
		GetLogger(g).Debug("endpoint-specific configuration found",
			zap.String("path", r.URL.Path))
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			allowedEntities = ecfg.AllowRead
		case http.MethodPost, http.MethodPut, http.MethodDelete:
			allowedEntities = ecfg.AllowWrite
		}
	}

	return g.AuthenticateIn(ctx, AllowedEntities(allowedEntities...), CallerName(extractCallerName(r)))
}

// extractCallerName returns name of the service sending the request as
// indicated by request headers. Empty string means unknown.
func extractCallerName(r *http.Request) string {
	if source := r.Header.Get(internal.CallerHeader); source != "" {
		return source
	}
	return r.Header.Get(internal.XUberSourceHeader)
}
