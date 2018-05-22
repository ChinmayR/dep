package handlers

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkassh"

	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

const (
	// lookupDuration Represents maximum time a lookup claim can be valid.
	lookupDuration = 5 * time.Second
)

type lookupHandler struct {
	log           *zap.Logger
	metrics       tally.Scope
	db            wonkadb.EntityDB
	eccPrivateKey *ecdsa.PrivateKey
	usshCAKeys    []ssh.PublicKey
	host          string
}

func (h lookupHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newLookupHandler returns a new handler that serves lookup http requests.
func newLookupHandler(cfg common.HandlerConfig) xhttp.Handler {
	h := lookupHandler{
		log:           cfg.Logger.With(zap.String("endpoint", "lookup")),
		metrics:       cfg.Metrics.Tagged(map[string]string{"endpoint": "lookup"}),
		db:            cfg.DB,
		eccPrivateKey: cfg.ECPrivKey,
		usshCAKeys:    cfg.Ussh,
		host:          cfg.Host,
	}
	return h
}

// Note: lookup replies aren't encrypted with the pubkey of the requestor.
// do we want to start encrypting them?
func validTime(ctime int, goodFor time.Duration) error {
	t := time.Unix(int64(ctime), 0)
	if t.Add(goodFor).After(time.Now()) {
		return nil
	}
	return fmt.Errorf("expired ctime: %s", t.String())
}

func (h lookupHandler) verifyLookupSignature(pubKey crypto.PublicKey, req wonka.LookupRequest) error {
	h.log.Debug("verifyLookupSignature")

	toVerify := []byte(fmt.Sprintf("%s<%d>%s", req.EntityName, req.Ctime, req.RequestedEntity))
	if req.Version == wonka.SignEverythingVersion {
		verifyReq := req
		verifyReq.Signature = ""
		verifyReq.USSHCertificate = ""
		verifyReq.USSHSignature = ""
		verifyReq.USSHSignatureType = ""

		var err error
		toVerify, err = json.Marshal(verifyReq)
		if err != nil {
			h.log.Error("json marshalling lookup request", zap.Error(err))
			return err
		}
	}

	if err := keys.VerifySignature(pubKey, req.Signature, req.SigType, string(toVerify)); err != nil {
		h.log.Error("signature doesn't validate", zap.Error(err))
		return err
	}

	return nil
}

func (h lookupHandler) processLookupRequest(ctx context.Context, req wonka.LookupRequest, w http.ResponseWriter) {
	// All logs should have the name of the entity who is making this lookup
	// request.
	h.log = h.log.With(
		zap.Any("entity", req.EntityName),
		zap.Any("requested_entity", req.RequestedEntity),
	)
	// first lookup the entity making the request. we do this so we can
	// verify the signature of the request.
	if req.USSHSignature != "" && req.USSHCertificate != "" {
		h.log.Debug("non-enrolled entity performing lookup")

		cert, err := wonkassh.CertFromRequest(req.USSHCertificate, h.usshCAKeys)
		if err != nil {
			writeResponse(w, h, err, wonka.LookupInvalidUSSHCert, http.StatusNotFound)
			return
		}

		toVerify := []byte(fmt.Sprintf("%s<%d>%s|%s", req.EntityName, req.Ctime, req.RequestedEntity,
			req.USSHCertificate))

		if req.Version == wonka.SignEverythingVersion {
			verifyReq := req
			verifyReq.USSHSignature = ""
			verifyReq.USSHSignatureType = ""
			toVerify, err = json.Marshal(verifyReq)
			if err != nil {
				writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
				return
			}
		}

		h.log.Debug("verify ussh signature", zap.Any("toVerify", toVerify))

		if err := wonkassh.VerifyUSSHSignature(cert, string(toVerify), req.USSHSignature,
			req.USSHSignatureType); err != nil {
			writeResponse(w, h, err, wonka.LookupInvalidUSSHSignature, http.StatusBadRequest)
			return
		}

		if err := validTime(req.Ctime, 30*time.Second); err != nil {
			writeResponse(w, h, err, wonka.LookupExpired, http.StatusBadRequest)
			return
		}
	} else {
		requestorEntity, err := h.db.Get(ctx, req.EntityName)
		if err != nil {
			writeResponseForWonkaDBError(w, h, err, "lookup")
			return
		}

		var pubKey crypto.PublicKey
		switch req.Version {
		case wonka.SignEverythingVersion:
			pubKey, err = wonka.KeyFromCompressed(requestorEntity.ECCPublicKey)
		default:
			pubKey, err = keys.ParsePublicKey(requestorEntity.PublicKey)
		}

		if err != nil {
			// since the public key comes from the database any problems are
			// server's fault.
			writeResponse(w, h, err, wonka.LookupServerError, http.StatusInternalServerError)
			return
		}

		if err := h.verifyLookupSignature(pubKey, req); err != nil {
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusForbidden)
			return
		}

		if req.Ctime != 0 {
			if err := validTime(req.Ctime, lookupDuration); err != nil {
				writeResponse(w, h, err, wonka.LookupExpired, http.StatusBadRequest)
				return
			}
		}
	}

	// Now lets try to lookup the registered entity
	requestedEntity, err := h.db.Get(ctx, req.RequestedEntity)
	if err != nil {
		writeResponseForWonkaDBError(w, h, err, "lookup")
		return
	}

	// Marshal the result entity entry to JSON
	replyBytes, err := json.Marshal(*requestedEntity)
	if err != nil {
		writeResponse(w, h, err, wonka.InternalError, http.StatusInternalServerError)
		return
	}

	sig, err := wonkacrypter.New().Sign(replyBytes, h.eccPrivateKey)
	if err != nil {
		writeResponse(w, h, err, wonka.InternalError, http.StatusInternalServerError)
		return
	}

	resp := wonka.LookupResponse{
		Result:    wonka.ResultOK,
		Entity:    *requestedEntity,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}

	writeResponse(w, h, nil, resp.Result, http.StatusOK, responseBody(resp))
}

func (h lookupHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	var req wonka.LookupRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
		return
	}
	h.processLookupRequest(ctx, req, w)
}
