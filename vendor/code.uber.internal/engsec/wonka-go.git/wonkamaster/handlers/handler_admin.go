package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/timehelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

const (
	// adminGroup is the group a user needs to be a member of in order to make
	// admin requests
	adminGroup = "wonka-admins"
)

var (
	allowedAdminSkew = 30 * time.Second
)

type adminHandler struct {
	log         *zap.Logger
	metrics     tally.Scope
	db          wonkadb.EntityDB
	pulloClient rpc.PulloClient
	usshCAKeys  []ssh.PublicKey
	host        string
}

func (h adminHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newAdminHandler returns a new handler that serves admin requests over http.
func newAdminHandler(cfg common.HandlerConfig) xhttp.Handler {
	h := adminHandler{
		log:         cfg.Logger.With(zap.String("endpoint", "admin")),
		metrics:     cfg.Metrics.Tagged(map[string]string{"endpoint": "admin"}),
		db:          cfg.DB,
		pulloClient: cfg.Pullo,
		usshCAKeys:  cfg.Ussh,
		host:        cfg.Host,
	}
	return h
}

func (h adminHandler) usshUserVerify(req wonka.AdminRequest, name string) error {
	certChecker := ssh.CertChecker{
		IsUserAuthority: func(k ssh.PublicKey) bool {
			for _, ca := range h.usshCAKeys {
				if bytes.Equal(k.Marshal(), ca.Marshal()) {
					return true
				}
			}
			return false
		},
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.Ussh))
	if err != nil {
		return fmt.Errorf("error parsing ssh key: %v", err)
	}

	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return errors.New("not an ssh certificate")
	}

	certName := strings.Split(name, "@")
	if len(certName) != 2 {
		return errors.New("invalid requestor name")
	}

	if err := certChecker.CheckCert(certName[0], cert); err != nil {
		return fmt.Errorf("certificate validation failed: %v", err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		return fmt.Errorf("error decoding signature: %v", err)
	}

	sig := &ssh.Signature{
		Blob:   sigBytes,
		Format: req.SignatureFormat,
	}

	verifyReq := req
	verifyReq.Signature = ""
	verifyReq.SignatureFormat = ""

	toVerify, err := json.Marshal(verifyReq)
	if err != nil {
		return fmt.Errorf("error marshalling request for signature verification: %v", err)
	}

	if err := cert.Verify(toVerify, sig); err != nil {
		return fmt.Errorf("ussh signature verification failed: %v", err)
	}
	return nil
}

// AdminHandler is the administrative interface.
func (h adminHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	if r.Method != http.MethodPost {
		writeResponse(w, h, errors.New("http method not allowed"), wonka.AdminInvalidCmd,
			http.StatusMethodNotAllowed)
		return
	}

	// if we're here, we should be authorized
	var req wonka.AdminRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
		return
	}

	h.log = h.log.With(
		zap.Any("action", req.Action),
		zap.Any("action_on", req.ActionOn),
	)
	// validate ussh signature
	if err := h.usshUserVerify(req, req.EntityName); err != nil {
		writeResponse(w, h, err, wonka.SignatureVerifyError,
			http.StatusForbidden)
		return
	}

	if !h.pulloClient.IsMemberOf(req.EntityName, adminGroup) {
		writeResponse(w, h, errors.New("admin user not permitted"), wonka.AdminAccessDenied,
			http.StatusForbidden)
		return
	}

	now := time.Now()
	createTime := time.Unix(req.Ctime, 0)
	if !timehelper.WithinClockSkew(createTime, now, allowedAdminSkew) {
		writeResponse(w, h, errTime, wonka.AdminAccessDenied, http.StatusForbidden)
		return
	}

	switch req.Action {
	case wonka.DeleteEntity:
		h.log = h.log.With(zap.Any("entity_to_delete", req.ActionOn))
		result, code, err := h.doDelete(ctx, req)
		if err != nil {
			writeResponse(w, h, err, result, code)
			return
		}
	}

	writeResponse(w, h, nil, wonka.ResultOK, http.StatusOK)
}

func (h adminHandler) doDelete(ctx context.Context, req wonka.AdminRequest) (string, int, error) {
	h.log.Info("request to delete entity")
	if err := h.db.Delete(ctx, req.ActionOn); err != nil {
		if err == wonkadb.ErrNotFound {
			return wonka.EntityUnknown, http.StatusNotFound, err
		}
		return wonka.LookupServerError, http.StatusInternalServerError, err
	}

	h.log.Info("requestor deleted entity")
	return wonka.ResultOK, http.StatusOK, nil
}
