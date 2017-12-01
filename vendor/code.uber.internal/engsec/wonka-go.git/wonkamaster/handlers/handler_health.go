package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

type healthHandler struct {
	log     *zap.Logger
	metrics tally.Scope
	host    string
}

func (h healthHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newHealthHandler returns a new handler that serves health check requests.
func newHealthHandler(cfg common.HandlerConfig) xhttp.Handler {
	h := healthHandler{
		log:     cfg.Logger.With(zap.Any("endpoint", "health")),
		metrics: cfg.Metrics.Tagged(map[string]string{"endpoint": "health"}),
		host:    cfg.Host,
	}
	return h
}

// HealthHandler returns ok
func (h healthHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log.Debug("health handler connection", zap.Any("addr", r.RemoteAddr))

	if strings.EqualFold(r.URL.Path, "/health/json") {
		// Metrics will be tagged result:OK for json responses
		writeResponse(w, h, nil, wonka.ResultOK, http.StatusOK)
		return
	}

	// Metrics will be tagged result:success for text responses
	tagSuccess(h.metrics)
	fmt.Fprintf(w, "OK\r\n")
}
