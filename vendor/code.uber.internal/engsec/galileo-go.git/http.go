package galileo

import (
	"context"
	"net/http"

	"go.uber.org/zap"
)

// AuthenticateHTTPRequest authenticates a request to a particular HTTP
// endpoint.
//
// It uses the endpoint-specific configuration for the given request, or the
// global configuration if no endpoint-specific configuration is found.
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

	return g.AuthenticateIn(ctx, allowedEntities)
}
