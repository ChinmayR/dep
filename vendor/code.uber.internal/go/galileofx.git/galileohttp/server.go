package galileohttp

import (
	"io"
	"net/http"
	"strings"

	galileo "code.uber.internal/engsec/galileo-go.git"
	"go.uber.org/zap"
)

const _unauthorizedMessage = "Request did not contain a valid authentication token. See http://t.uber.com/galileo-onboarding"

// AuthenticateInOption customizes the behavior of the AuthenticateIn
// middleware.
type AuthenticateInOption interface {
	applyInOption(*authenticateInCfg)
}

type authenticateInOptionFunc func(*authenticateInCfg)

func (f authenticateInOptionFunc) applyInOption(cfg *authenticateInCfg) { f(cfg) }

type authenticateInCfg struct {
	Logger       *zap.Logger
	IgnoredPaths map[string]struct{}
}

// AuthenticateInLogger specifies the logger to be used by the
// AuthenticateInMiddleware when a request gets rejected.
//
// This defaults to the logger associated with the Galileo object (if any), or
// the global Zap logger.
func AuthenticateInLogger(log *zap.Logger) AuthenticateInOption {
	return authenticateInOptionFunc(func(cfg *authenticateInCfg) {
		cfg.Logger = log
	})
}

// AuthenticateInIgnorePaths specifies HTTP URL paths for which authentication
// will not be attempted.
//
//   AuthenticateInMiddleware(g, AuthenticateInIgnorePaths("/health", "/debug/pprof"))
//
// All requests to the specified paths will be allowed through without calling
// Galileo.
func AuthenticateInIgnorePaths(paths ...string) AuthenticateInOption {
	return authenticateInOptionFunc(func(cfg *authenticateInCfg) {
		for _, p := range paths {
			// Strip trailing '/'
			if p[len(p)-1] == '/' {
				p = p[:len(p)-1]
			}
			cfg.IgnoredPaths[p] = struct{}{}
		}
	})
}

// AuthenticateInMiddleware builds a middleware that authenticates all
// incoming HTTP requests with the given Galileo client. Bad requests are
// rejected with an HTTP 403 error.
//
// The returned middleware expects the http.Handler to have Jaeger tracing
// support.
func AuthenticateInMiddleware(g galileo.Galileo, opts ...AuthenticateInOption) func(http.Handler) http.Handler {
	cfg := authenticateInCfg{
		Logger:       galileo.GetLogger(g),
		IgnoredPaths: make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt.applyInOption(&cfg)
	}

	return func(h http.Handler) http.Handler {
		return authHandler{
			g:            g,
			handler:      h,
			log:          cfg.Logger,
			ignoredPaths: cfg.IgnoredPaths,
		}
	}
}

type authHandler struct {
	g            galileo.Galileo
	handler      http.Handler
	log          *zap.Logger
	ignoredPaths map[string]struct{}
}

func (h *authHandler) shouldIgnore(p string) bool {
	// Strip trailing '/'
	if p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	_, shouldIgnore := h.ignoredPaths[p]
	return shouldIgnore
}

func (h authHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.shouldIgnore(r.URL.Path) {
		h.handler.ServeHTTP(w, r)
		return
	}

	// TODO(abg): We'll want to expose per-endpoint configuration via options
	// somehow. Galileo Go provides a notion of per-endpoint configurations
	// with concepts of read and write based on the HTTP method name.
	//
	// Unfortunately, that concept of read/write doesn't leave room for RPC
	// systems that integrate with Galileo. We need to figure out an
	// alternative configuration format that allows both, HTTP and RPC
	// endpoints to configure per-endpoint overrides for Galileo
	// configuration.
	//
	// Alternatively, we can just use Galileo's existing configuration format
	// with the caveat that it's HTTP-only.
	err := h.g.AuthenticateIn(r.Context())
	if err == nil {
		h.handler.ServeHTTP(w, r)
		return
	}

	allowed := galileo.GetAllowedEntities(err)
	h.log.Error("unauthorized request",
		zap.Error(err),
		zap.String("remote", r.RemoteAddr),
		zap.String("endpoint", r.URL.Path),
		zap.String("method", r.Method),
		zap.Strings("allowed_entities", allowed),
	)

	w.Header().Set(galileo.WonkaRequiresHeader, strings.Join(allowed, ","))
	w.WriteHeader(http.StatusForbidden)
	io.WriteString(w, _unauthorizedMessage)
}
