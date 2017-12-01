package handlers

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
)

type destroyHandler struct {
	log     *zap.Logger
	metrics tally.Scope
	db      wonkadb.EntityDB
	host    string
}

func (h destroyHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newDestroyHandler returns a new handler that serves http requests to destroy
// entities
func newDestroyHandler(cfg common.HandlerConfig) xhttp.Handler {
	h := destroyHandler{
		log:     cfg.Logger.With(zap.String("endpoint", "destroy")),
		metrics: cfg.Metrics.Tagged(map[string]string{"endpoint": "destroy"}),
		db:      cfg.DB,
		host:    cfg.Host,
	}

	return h
}

// DestroyHandler destroys an entity
func (h destroyHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	// Get the entity name from the querystring or bail
	entityName := req.URL.Query().Get("id")
	if entityName == "" {
		writeResponse(w, h, errors.New("missing entity name"), wonka.EntityUnknown, http.StatusBadRequest)
		return
	}

	h.log = h.log.With(zap.String("entity", entityName))
	// Does the requested ENTITY_NAME exist? if not bail out now
	dbe, err := h.db.Get(ctx, entityName)
	if err != nil {
		writeResponseForWonkaDBError(w, h, err, "lookup")
		return
	}

	// Convert publicKey to rsa.PublicKey
	rsaPubKey, err := keys.ParsePublicKey(dbe.PublicKey)
	if err != nil {
		writeResponse(w, h, err, wonka.InvalidPublicKey, http.StatusInternalServerError)
		return
	}

	// Build the entity_name<ctime>cmd deletion challenge/request string
	verifyMe := fmt.Sprintf("%s<%d>DESTROY_ENTITY", dbe.EntityName, dbe.CreateTime.Unix())

	destroySigBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		writeResponse(w, h, err, wonka.ResultRejected, http.StatusInternalServerError)
		return
	}
	destroySig := string(destroySigBytes)

	if err := keys.VerifySignature(rsaPubKey, destroySig, "SHA256", verifyMe); err != nil {
		writeResponse(w, h, err, wonka.SignatureVerifyError, http.StatusForbidden)
		return
	}

	// Now lets try to delete the named entity
	if err := h.db.Delete(ctx, dbe.EntityName); err != nil {
		writeResponseForWonkaDBError(w, h, err, "delete")
		return
	}

	writeResponse(w, h, nil, wonka.ResultOK, http.StatusOK)
}
