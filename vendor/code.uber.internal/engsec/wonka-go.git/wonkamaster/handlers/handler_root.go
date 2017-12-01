package handlers

import (
	"context"
	"fmt"
	"net/http"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

type rootHandler struct {
	log     *zap.Logger
	metrics tally.Scope
}

// NewRootHandler returns a new handler that serves requests to root url.
func NewRootHandler(cfg common.HandlerConfig) xhttp.Handler {
	h := rootHandler{
		log:     cfg.Logger.With(zap.Any("endpoint", "root")),
		metrics: cfg.Metrics.Tagged(map[string]string{"endpoint": "root"}),
	}
	return h
}

// rootHandler - for fetches of /
func (h rootHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()

	h.log.Debug("root handler connection", zap.Any("addr", r.RemoteAddr))
	tagSuccess(h.metrics)

	fmt.Fprintf(w, "\r\n")
}
