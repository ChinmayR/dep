package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"

	"golang.org/x/time/rate"
)

type rateLimiter struct {
	g     *rate.Limiter
	paths map[string]*rate.Limiter
}

// RateConfig defines rate limiting configuration, which applied on a
// per-server basis.
type RateConfig struct {
	Global    *RateSpec            `yaml:"global"`
	Endpoints []RateEndpointConfig `yaml:"endpoints"`
}

// RateEndpointConfig defines the rate limiting configuration scoped
// to a given endpoint.
type RateEndpointConfig struct {
	Path string
	R    float64 `yaml:"events_per_second"`
	B    int     `yaml:"burst_limit"`
}

// RateSpec defines how a particular scope should be rate limited.
type RateSpec struct {
	// R is the rate in terms of events per second.
	R float64 `yaml:"events_per_second"`

	// B is the burst parameter, i.e. the capacity of the token bucket.
	B int `yaml:"burst_limit"`
}

// NewRateLimiter creates a rate limiting middleware that returns HTTP 429 to
// the caller when the limit is exceeded.
func NewRateLimiter(c RateConfig) xhttp.Filter {
	r := &rateLimiter{
		paths: make(map[string]*rate.Limiter),
	}
	if c.Global != nil {
		r.g = rate.NewLimiter(rate.Limit(c.Global.R), c.Global.B)
	}
	for _, e := range c.Endpoints {
		path := strings.ToLower(strings.TrimSpace(e.Path))
		r.paths[path] = rate.NewLimiter(rate.Limit(e.R), e.B)
	}
	return r
}

func (b *rateLimiter) Apply(ctx context.Context, w http.ResponseWriter, r *http.Request, next xhttp.Handler) {
	// Check the global limiter
	if g := b.g; g != nil && !g.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Too many requests"))
		return
	}

	// Check the endpoint limiter
	if b.blockEndpoint(r) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(fmt.Sprintf("Too many requests to the '%s' endpoint", r.URL.Path)))
		return
	}

	// Process the request
	next.ServeHTTP(ctx, w, r)
}

func (b *rateLimiter) blockEndpoint(r *http.Request) bool {
	if len(b.paths) == 0 {
		return false
	}

	if r == nil || r.URL == nil {
		return false
	}

	path := strings.ToLower(r.URL.Path)
	if l, ok := b.paths[path]; ok {
		return !l.Allow()
	}

	return false
}
