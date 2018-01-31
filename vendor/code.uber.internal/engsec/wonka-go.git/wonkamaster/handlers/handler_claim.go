package handlers

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/claimhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/timehelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkassh"

	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

const (
	// maxClaimTime Represents how long a claim is valid.
	maxClaimTime = 24 * time.Hour

	// allowedClaimSkew Represents allowed smear time around claim expiration in case of clock skew.
	allowedClaimSkew = 2 * time.Minute
)

type claimRequestType int

const (
	userClaim claimRequestType = iota
	serviceClaim
	hostClaim
	invalidClaim
)

type claimHandler struct {
	log                 *zap.Logger
	metrics             tally.Scope
	db                  wonkadb.EntityDB
	eccPrivateKey       *ecdsa.PrivateKey
	pulloClient         rpc.PulloClient
	usshCAKeys          []ssh.PublicKey
	usshHostKeyCallback ssh.HostKeyCallback
	impersonators       map[string]struct{}
	host                string
	certAuth            *common.CertAuthOverride
}

func (h claimHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newClaimHandler returns a new handler that serves claim requests over http.
func newClaimHandler(cfg common.HandlerConfig) xhttp.Handler {
	i := make(map[string]struct{}, len(cfg.Imp))

	for _, h := range cfg.Imp {
		i[h] = struct{}{}
	}

	h := claimHandler{
		log:                 cfg.Logger.With(zap.String("endpoint", "claim")),
		metrics:             cfg.Metrics.Tagged(map[string]string{"endpoint": "claim"}),
		db:                  cfg.DB,
		eccPrivateKey:       cfg.ECPrivKey,
		pulloClient:         cfg.Pullo,
		usshCAKeys:          cfg.Ussh,
		usshHostKeyCallback: cfg.UsshHostSigner,
		impersonators:       i,
		host:                cfg.Host,
		certAuth:            cfg.CertAuthenticationOverride,
	}

	return h
}

func (h claimHandler) authorizeImpersonation(req *wonka.ClaimRequest) error {
	if _, ok := h.impersonators[req.EntityName]; !ok {
		return errors.New("unauthorized impersonator")
	}

	if req.ImpersonatedEntity == req.EntityName {
		return errors.New("impersonater == impersonatee")
	}

	req.EntityName = req.ImpersonatedEntity
	return nil
}

// ClaimHandler is the endpoint for retreiving claims.
func (h claimHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	var req wonka.ClaimRequest
	// Parse json claim message into c
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
		return
	}

	// All logs should have the name of the entity who is making this claim
	// request.
	h.log = h.log.With(
		zap.String("entity", req.EntityName),
		zap.String("claims_requested", req.Claim))

	var pubKey crypto.PublicKey
	var err error
	reqType := serviceClaim

	if len(req.Certificate) != 0 {
		// do certificate auth here
		cert, err := authenticateCertificate(req.Certificate, req.EntityName, h.certAuth, h.log)
		if err != nil {
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusForbidden)
			return
		}

		pubKey, err = cert.PublicKey()
		if err != nil {
			tagError(h.metrics, err)
			err = fmt.Errorf("error extracting public key from certificate: %v", err)
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusForbidden)
			return
		}

		h.log = h.log.With(
			zap.String("authtype", "WonkaCertificate"),
			zap.String("host", cert.Host),
			zap.Int64("serial", int64(cert.Serial)),
			zap.String("runtime", cert.Tags[wonka.TagRuntime]),
			zap.String("taskid", cert.Tags[wonka.TagTaskID]))
		if cert.Type == wonka.EntityTypeUser {
			h.log.Debug("wonka certificate for user claim request")
			reqType = userClaim
		}
	} else if req.USSHSignature != "" {
		var cert *ssh.Certificate
		cert, pubKey, reqType, err = h.usshAuth(&req)
		if err != nil {
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusForbidden)
			return
		}

		certType := "USSHHostCert"
		if cert.CertType == ssh.UserCert {
			certType = "USSHUserCert"
		}
		h.log = h.log.With(
			zap.String("principal", cert.ValidPrincipals[0]),
			zap.Int64("ussh_serial", int64(cert.Serial)),
			zap.String("authtype", certType))
	} else {
		// Find the source entity or fail out of the request now
		dbe, err := h.db.Get(ctx, req.EntityName)
		if err != nil {
			writeResponseForWonkaDBError(w, h, err, "lookup")
			return
		}

		pubKey, err = wonka.KeyFromCompressed(dbe.ECCPublicKey)
		if err != nil {
			writeResponse(w, h, err, wonka.ResultRejected, http.StatusInternalServerError)
			return
		}

		if err := h.verifyClaimRequest(req, pubKey.(*ecdsa.PublicKey)); err != nil {
			writeResponse(w, h, err, err.Error(), http.StatusForbidden)
			return
		}

		h.log = h.log.With(zap.String("authtype", "enrolled"))
	}

	if req.ImpersonatedEntity != "" {
		h.log = h.log.With(zap.String("impersonated_entity", req.ImpersonatedEntity))
		if err := h.authorizeImpersonation(&req); err != nil {
			writeResponse(w, h, err, wonka.ClaimInvalidImpersonator, http.StatusBadRequest)
			return
		}
		// if we have an authorized impersonation, the request type is whatever the impersonated
		// entity is.
		if isPersonnelClaim(req.EntityName) {
			reqType = userClaim
		}
	}

	req.CreateTime = time.Unix(req.Ctime, 0)
	req.ExpireTime = time.Unix(req.Etime, 0)
	// is the claim expired?
	currentTime := time.Now()
	// you can request a claim with a cTime that is allowedClaimSkew in the future
	// or the past. In practice, this means that the requestors clock can be
	// either 2 minutes ahead or behind our own.
	if !timehelper.WithinClockSkew(req.CreateTime, currentTime, allowedClaimSkew) {
		writeResponse(w, h, errTime, wonka.ErrTimeWindow, http.StatusForbidden)
		return
	}

	if currentTime.After(req.ExpireTime) {
		writeResponse(w, h, errTime, wonka.ClaimRequestExpired, http.StatusForbidden)
		return
	}

	maxEtime := time.Now().Add(maxClaimTime)
	if req.ExpireTime.After(maxEtime) {
		h.log.Warn("claim etime truncated",
			zap.Any("expire_time", req.ExpireTime),
			zap.Any("max_claim_time", maxClaimTime.String()),
		)

		req.ExpireTime = maxEtime
		req.Etime = int64(req.ExpireTime.Unix())
	}

	if req.Destination == "" {
		req.Destination = req.EntityName
	}

	approvedClaims, err := validClaimsFromRequest(ctx, h.log, h.pulloClient, req, reqType)
	if err != nil {
		h.log.Error("no allowed claims found")
		writeResponse(w, h, fmt.Errorf("http://t.uber.com/galileo-onboarding, %v", err),
			wonka.ClaimRejectedNoAccess, http.StatusForbidden)
		return
	}
	req.Claim = approvedClaims

	claim, err := claimhelper.NewSignedClaim(req, h.eccPrivateKey)
	if err != nil {
		fmt.Printf("err %v\n", err)
		writeResponse(w, h, err, wonka.ClaimSigningError, http.StatusInternalServerError)
		return
	}

	encryptedToken, err := claimhelper.EncryptClaim(claim, h.eccPrivateKey, pubKey)
	if err != nil {
		fmt.Printf("err here %v\n", err)
		writeResponse(w, h, err, wonka.ClaimSigningError, http.StatusInternalServerError)
		return
	}

	h.log = h.log.With(zap.String("destination", req.Destination),
		zap.String("approved_claims", req.Claim))

	resp := wonka.ClaimResponse{
		Result: wonka.ResultOK,
		Token:  encryptedToken,
	}

	writeResponse(w, h, nil, resp.Result, http.StatusOK, responseBody(resp))
}

func isEntityInDeprecatedGroup(claimRequested, requestingEntity string) bool {
	if strings.EqualFold(wonka.EveryEntity, claimRequested) {
		return true
	}
	if !strings.EqualFold("knoxgroup", claimRequested) {
		return false
	}
	return strings.EqualFold(requestingEntity, "hadoop-gw") ||
		strings.EqualFold(requestingEntity, "knox") ||
		strings.EqualFold(requestingEntity, "querybuilder") ||
		strings.EqualFold(requestingEntity, "michelangelo") ||
		strings.EqualFold(requestingEntity, "queryrunner") ||
		strings.EqualFold(requestingEntity, "michelangelo-rest")
}

func (h claimHandler) verifyClaimRequest(cr wonka.ClaimRequest, pubKey *ecdsa.PublicKey) error {
	toVerify := cr
	toVerify.Signature = ""
	toVerify.USSHCertificate = ""
	toVerify.USSHSignature = ""
	toVerify.USSHSignatureType = ""

	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling claim request to verify: %v", err)
	}

	sig, err := base64.StdEncoding.DecodeString(cr.Signature)
	if err != nil {
		return fmt.Errorf("error base64 decoding signature: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerifyBytes, sig, pubKey); !ok {
		return fmt.Errorf("claim request signature doesn't verify")
	}

	return nil
}

// userAuth validates the ussh signature on the claim request, if present.
func (h claimHandler) usshAuth(c *wonka.ClaimRequest) (*ssh.Certificate, crypto.PublicKey, claimRequestType, error) {
	var dbe wonka.Entity
	dbe.EntityName = c.EntityName
	dbe.CreateTime = time.Unix(c.Ctime, 0)
	dbe.ExpireTime = time.Unix(c.Etime, 0)
	dbe.PublicKey = c.SessionPubKey

	h.log.Info("user auth test",
		zap.Any("entity", c.EntityName),
		zap.Any("claim", c.Claim),
	)

	pubKey, err := wonka.KeyFromCompressed(c.SessionPubKey)
	if err != nil {
		return nil, nil, invalidClaim, fmt.Errorf("error parsing ecc pubkey: %v", err)
	}

	if err := h.verifyClaimRequest(*c, pubKey); err != nil {
		return nil, nil, invalidClaim, fmt.Errorf("validating inner claim failed: %v", err)
	}

	h.log.Debug("claim request verifies, checking ussh",
		zap.Any("entity", c.EntityName),
	)

	sshPubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.USSHCertificate))
	if err != nil {
		return nil, nil, invalidClaim, fmt.Errorf("parsing ssh key failed: %v", err)
	}

	cert, ok := sshPubKey.(*ssh.Certificate)
	if !ok {
		return nil, nil, invalidClaim, errors.New("rejecting non-certificate key")
	}

	var reqType claimRequestType
	var verify func(*wonka.ClaimRequest, *ssh.Certificate) error
	switch cert.CertType {
	case ssh.UserCert:
		verify = h.usshUserVerify
		reqType = userClaim
	case ssh.HostCert:
		verify = h.usshHostVerify
		reqType = hostClaim
	}

	err = verify(c, cert)
	if err != nil {
		return nil, nil, invalidClaim, fmt.Errorf("ussh verify failure: %v", err)
	}

	return cert, pubKey, reqType, nil
}

func (h claimHandler) verifyUssh(cr *wonka.ClaimRequest, cert *ssh.Certificate) error {
	toVerify := *cr
	toVerify.USSHSignature = ""
	toVerify.USSHSignatureType = ""

	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("json marshal failure: %v", err)
	}

	err = wonkassh.VerifyUSSHSignature(cert, string(toVerifyBytes), cr.USSHSignature, cr.USSHSignatureType)
	if err != nil {
		return fmt.Errorf("ussh signature check failed: %v", err)
	}

	return nil
}

func (h claimHandler) usshUserVerify(cr *wonka.ClaimRequest, cert *ssh.Certificate) error {
	certName := strings.Split(cr.EntityName, "@")
	if len(certName) != 2 {
		return errors.New("wonkamaster: invalid personnel entity name. http://t.uber.com/wm-ipen")
	}

	if err := wonkassh.CheckUserCert(certName[0], cert, h.usshCAKeys); err != nil {
		return err
	}

	err := h.verifyUssh(cr, cert)
	if err != nil {
		return fmt.Errorf("user verify: %v", err)
	}

	// this only matters if the claim request is valid, but there's no harm in
	// doing this unconditionally.
	if cr.Etime == 0 {
		cr.Etime = int64(cert.ValidBefore)
	}

	return nil
}

func (h claimHandler) usshHostVerify(cr *wonka.ClaimRequest, cert *ssh.Certificate) error {
	// using EntityName from the claim request means that the EntityName needs to be fully qualified.
	if cr.EntityName == "localhost" {
		return errors.New("invalid entity name")
	}

	name := fmt.Sprintf("%s:22", cr.EntityName)
	addr := &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}

	if err := h.usshHostKeyCallback(name, addr, cert); err != nil {
		h.log.Error("error validating host cert",
			zap.Any("entity", cr.EntityName),
			zap.Error(err),
			zap.Any("signing_key", ssh.FingerprintSHA256(cert.SignatureKey)),
		)

		return fmt.Errorf("error validating host cert: %v", err)
	}

	// host cert is valid, now let's verify that the private key signed the rest of the message\
	if err := h.verifyUssh(cr, cert); err != nil {
		return fmt.Errorf("host verify: %v", err)
	}

	return nil
}
