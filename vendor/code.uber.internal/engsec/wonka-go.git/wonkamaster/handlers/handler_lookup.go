package handlers

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	certAuth      *common.CertAuthOverride
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
		certAuth:      cfg.CertAuthenticationOverride,
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

	h.log.Debug("trying to verify lookup signature")
	if err := keys.VerifySignature(pubKey, req.Signature, req.SigType, string(toVerify)); err != nil {
		h.log.Error("signature doesn't validate", zap.Error(err))
		return err
	}

	return nil
}

func (h lookupHandler) verifyWonkaCertSignature(req wonka.LookupRequest) error {
	toVerify := req
	toVerify.Signature = ""
	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling request to verify: %v", err)
	}

	sig, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		return fmt.Errorf("error unmarshalling signature: %v", err)
	}

	cert, err := authenticateCertificate(req.Certificate, req.EntityName, h.certAuth, h.log)
	if err != nil {
		return err
	}

	pubKey, err := cert.PublicKey()
	if err != nil {
		return fmt.Errorf("error extracting public key from certificate: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerifyBytes, sig, pubKey); !ok {
		return errors.New("certificate signature does not verify")
	}

	h.log.Debug("certificate lookup signature is valid")

	return nil
}

func (h lookupHandler) verifyUsshSignature(req wonka.LookupRequest) error {
	h.log.Debug("non-enrolled entity performing lookup")

	cert, err := wonkassh.CertFromRequest(req.USSHCertificate, h.usshCAKeys)
	if err != nil {
		return errors.New(wonka.LookupInvalidUSSHCert)
	}

	toVerify := []byte(fmt.Sprintf("%s<%d>%s|%s", req.EntityName, req.Ctime, req.RequestedEntity,
		req.USSHCertificate))

	if req.Version == wonka.SignEverythingVersion {
		verifyReq := req
		verifyReq.USSHSignature = ""
		verifyReq.USSHSignatureType = ""
		toVerify, err = json.Marshal(verifyReq)
		if err != nil {
			return errors.New(wonka.DecodeError)
		}
	}

	h.log.Debug("verify ussh signature", zap.Any("toVerify", toVerify))

	err = wonkassh.VerifyUSSHSignature(cert, string(toVerify), req.USSHSignature, req.USSHSignatureType)
	if err != nil {
		return errors.New(wonka.LookupInvalidUSSHSignature)
	}

	return validTime(req.Ctime, 30*time.Second)
}

func (h lookupHandler) verifyEnrolled(ctx context.Context, req wonka.LookupRequest) error {
	requestorEntity, err := h.db.Get(ctx, req.EntityName)
	if err != nil {
		return errors.New(wonka.LookupEntityUnknown)
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
		return errors.New(wonka.LookupServerError)
	}

	if err := h.verifyLookupSignature(pubKey, req); err != nil {
		return errors.New(wonka.ResultRejected)
	}

	if err := validTime(req.Ctime, lookupDuration); err != nil {
		return errors.New(wonka.LookupExpired)
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
	if len(req.Certificate) > 0 {
		if err := h.verifyWonkaCertSignature(req); err != nil {
			writeResponse(w, h, err, wonka.LookupInvalidSignature, http.StatusBadRequest)
			return
		}
	} else if req.USSHSignature != "" && req.USSHCertificate != "" {
		if err := h.verifyUsshSignature(req); err != nil {
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusBadRequest)
			return
		}
	} else {
		if err := h.verifyEnrolled(ctx, req); err != nil {
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusBadRequest)
			return
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
