package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/claims"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkassh"

	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type resolveHandler struct {
	log           *zap.Logger
	metrics       tally.Scope
	db            wonkadb.EntityDB
	pulloClient   rpc.PulloClient
	host          string
	eccPrivateKey *ecdsa.PrivateKey
	usshCAKeys    []ssh.PublicKey
}

func (h resolveHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

func newResolveHandler(cfg common.HandlerConfig) xhttp.Handler {
	return resolveHandler{
		log:           cfg.Logger.With(zap.String("endpoint", "resolve")),
		metrics:       cfg.Metrics.Tagged(map[string]string{"endpoint": "resolve"}),
		host:          cfg.Host,
		eccPrivateKey: cfg.ECPrivKey,
		db:            cfg.DB,
		pulloClient:   cfg.Pullo,
		usshCAKeys:    cfg.Ussh,
	}
}

func (h resolveHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	var req wonka.ResolveRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
		return
	}

	h.log = h.log.With(zap.Any("entity", req.EntityName),
		zap.Any("requested_entity", req.RequestedEntity))

	// do auth on the request here.
	pubKey, claimType, err := h.authRequest(req)
	if err != nil {
		h.log.Error("auth failure",
			zap.Error(err),
			zap.Any("ussh", len(req.USSHCertificate)),
			zap.Any("certificate", len(req.Certificate)))

		writeResponse(w, h, err, wonka.SignatureVerifyError, http.StatusBadRequest)
		return
	}

	h.log.Debug("good resolve request", zap.Any("entity", req.EntityName),
		zap.Any("requesting", req.RequestedEntity))

	claimsToRequest := []string{wonka.EveryEntity, req.EntityName}
	if req.Claims != "" {
		claimsToRequest = append(claimsToRequest, strings.Split(req.Claims, ",")...)
	}

	// look up the naemd entity here.
	if e, err := h.db.Get(ctx, req.RequestedEntity); err == nil {
		claimsToRequest = append(claimsToRequest, strings.Split(e.Requires, ",")...)
	}

	etime := time.Unix(req.Etime, 0)
	if etime == time.Unix(0, 0) {
		etime = time.Now().Add(2 * time.Hour)
	}

	if etime.After(time.Now().Add(maxClaimTime)) {
		h.log.Warn("capped overlog etime", zap.Any("etime", etime.String()))
		etime = time.Now().Add(2 * time.Hour)
	}

	claimReq := wonka.ClaimRequest{
		EntityName:  req.EntityName,
		Claim:       strings.Join(claimsToRequest, ","),
		Destination: req.RequestedEntity,
		Ctime:       int64(time.Now().Add(-5 * time.Minute).Unix()),
		Etime:       int64(etime.Unix()),
	}

	claimReq.Claim, err = validClaimsFromRequest(ctx, h.log, h.pulloClient,
		claimReq, claimType)
	if err != nil {
		// this should never happen. an entity should always be able to request
		// an everyone or an identity claim, and both of those are part of the request
		writeResponse(w, h, err, wonka.ClaimRejectedNoAccess, http.StatusForbidden)
		return
	}

	h.log = h.log.With(zap.String("destination", claimReq.Destination),
		zap.String("claim", claimReq.Claim))

	// respond with a claim here.
	encryptedToken, err := claims.NewSignedClaim(claimReq, h.eccPrivateKey, pubKey)
	if err != nil {
		writeResponse(w, h, err, wonka.ClaimSigningError, http.StatusInternalServerError)
		return
	}

	resp := wonka.ClaimResponse{
		Result: wonka.ResultOK,
		Token:  encryptedToken,
	}

	writeResponse(w, h, nil, wonka.ResultOK, http.StatusOK, responseBody(resp))
}

func (h resolveHandler) authRequest(req wonka.ResolveRequest) (*ecdsa.PublicKey, claimRequestType, error) {
	var authFunc func(wonka.ResolveRequest, *ecdsa.PublicKey) (claimRequestType, error)

	if len(req.USSHCertificate) != 0 {
		authFunc = h.authUssh
	} else if len(req.Certificate) != 0 {
		authFunc = h.authCertificate
	} else {
		authFunc = h.authEnrolledEntity
	}

	pubKey, err := wonka.KeyFromCompressed(req.PublicKey)
	if err != nil {
		return nil, invalidClaim, fmt.Errorf("bad publickey: %v", err)
	}

	requestType, err := authFunc(req, pubKey)
	return pubKey, requestType, err
}

func (h resolveHandler) authEnrolledEntity(req wonka.ResolveRequest, pubKey *ecdsa.PublicKey) (claimRequestType, error) {
	pubKey, err := wonka.KeyFromCompressed(req.PublicKey)
	if err != nil {
		return invalidClaim, fmt.Errorf("error pulling out key: %v", err)
	}

	verifyReq := req
	verifyReq.Signature = nil
	toVerify, err := json.Marshal(verifyReq)
	if err != nil {
		return invalidClaim, fmt.Errorf("error marshalling for verification: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerify, req.Signature, pubKey); ok {
		return serviceClaim, nil
	}

	return invalidClaim, errors.New("signature verification error")
}

func (h resolveHandler) authUssh(req wonka.ResolveRequest, pubKey *ecdsa.PublicKey) (claimRequestType, error) {
	cert, err := wonkassh.CertFromRequest(string(req.USSHCertificate), h.usshCAKeys)
	if err != nil {
		return invalidClaim, fmt.Errorf("error pulling out certificate: %v", err)
	}

	// Check the USSH certificate against the CA for validity
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

	certName := strings.Split(req.EntityName, "@")
	if len(certName) != 2 {
		return invalidClaim, errors.New("wonkamaster: invalid personnel entity name. http://t.uber.com/wm-ipen")
	}

	if err := certChecker.CheckCert(certName[0], cert); err != nil {
		return invalidClaim, fmt.Errorf("ssh certcheck failure: %v", err)
	}

	verifyReq := req
	verifyReq.Signature = nil
	verifyReq.USSHSignatureType = ""
	toVerify, err := json.Marshal(verifyReq)
	if err != nil {
		return invalidClaim, fmt.Errorf("error marshalling for verification: %v", err)
	}

	sshSig := &ssh.Signature{
		Format: req.USSHSignatureType,
		Blob:   req.Signature,
	}

	if err := cert.Key.Verify([]byte(toVerify), sshSig); err != nil {
		return invalidClaim, fmt.Errorf("error ussh signature does not verify: %v", err)
	}

	requestType := userClaim
	if cert.CertType == ssh.HostCert {
		requestType = hostClaim
	}

	// auth ussh certificate here
	return requestType, nil
}

func (h resolveHandler) authCertificate(req wonka.ResolveRequest, pubKey *ecdsa.PublicKey) (claimRequestType, error) {
	cert, err := wonka.UnmarshalCertificate(req.Certificate)
	if err != nil {
		return invalidClaim, fmt.Errorf("error pulling out certificate: %v", err)
	}

	if err := cert.CheckCertificate(); err != nil {
		return invalidClaim, fmt.Errorf("invalid certificate: %v", err)
	}

	verifyReq := req
	verifyReq.Signature = nil
	toVerify, err := json.Marshal(verifyReq)
	if err != nil {
		return invalidClaim, fmt.Errorf("error marshalling for verification: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerify, req.Signature, pubKey); !ok {
		return invalidClaim, fmt.Errorf("error certificate signature doesn't verify")
	}

	requestType := serviceClaim
	if strings.HasSuffix(cert.EntityName, ".prod.uber.internal") {
		requestType = hostClaim
	}

	return requestType, nil
}
