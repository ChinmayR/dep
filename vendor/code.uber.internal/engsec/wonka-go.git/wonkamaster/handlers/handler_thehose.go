package handlers

// it sends wonka tokens or it gets the hose

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
)

type hoseHandler struct {
	derelicts map[string]time.Time

	//
	eccPriv       *ecdsa.PrivateKey
	log           *zap.Logger
	metrics       tally.Scope
	host          string
	checkInterval int
}

func (h hoseHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

func newHoseHandler(cfg common.HandlerConfig) xhttp.Handler {
	derelicts := make(map[string]time.Time, len(cfg.Derelicts))
	for k, v := range cfg.Derelicts {
		name := wonka.CanonicalEntityName(k)
		if expTime, err := time.Parse("2006/01/02", v); err == nil {
			derelicts[name] = expTime
		} else {
			cfg.Logger.Warn("ignoring service with invalid time specifcation",
				zap.Any("service", name),
				zap.Error(err))
			continue
		}
	}

	return hoseHandler{
		derelicts:     derelicts,
		eccPriv:       cfg.ECPrivKey,
		log:           cfg.Logger.With(zap.String("endpoint", "thehose")),
		metrics:       cfg.Metrics.Tagged(map[string]string{"endpoint": "thehose"}),
		checkInterval: cfg.HoseCheckInterval,
		host:          cfg.Host,
	}
}

func (h hoseHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	var req wonka.TheHoseRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err == nil {
		h.log = h.log.With(zap.String("entity", req.EntityName))
	}

	// we just serve the status reply to all requestors
	reply := wonka.TheHoseReply{
		CurrentStatus: "ok",
		CurrentTime:   time.Now().Unix(),
		Derelicts:     h.derelicts,
		CheckInterval: h.checkInterval,
	}

	h.log = h.log.With(jaegerzap.Trace(ctx))

	toSign, err := json.Marshal(reply)
	if err != nil {
		writeResponse(w, h, err, wonka.InternalError, http.StatusInternalServerError)
		return
	}

	// the client doesn't actually check this right now.
	reply.Signature, err = wonkacrypter.New().Sign(toSign, h.eccPriv)
	if err != nil {
		writeResponse(w, h, err, wonka.InternalError, http.StatusInternalServerError)
		return
	}

	writeResponse(w, h, nil, wonka.ResultOK, http.StatusOK, responseBody(reply))
}
