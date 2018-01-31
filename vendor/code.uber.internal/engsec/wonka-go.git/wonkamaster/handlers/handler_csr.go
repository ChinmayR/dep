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
	"reflect"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
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

var (
	allowedClockSkew   = time.Minute
	maxCertificateTime = 20 * time.Hour
)

const _uberHostSuffix = ".prod.uber.internal"

type csrHandler struct {
	eccPrivateKey       *ecdsa.PrivateKey
	db                  wonkadb.EntityDB
	log                 *zap.Logger
	metrics             tally.Scope
	usshHostKeyCallback ssh.HostKeyCallback
	usshUserKeys        []ssh.PublicKey
	host                string
	launchers           map[string]common.Launcher
}

func (h csrHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newCSRHandler returns a new CSR handler
func newCSRHandler(cfg common.HandlerConfig) xhttp.Handler {
	return csrHandler{
		eccPrivateKey:       cfg.ECPrivKey,
		db:                  cfg.DB,
		log:                 cfg.Logger.With(zap.String("endpoint", "csr")),
		metrics:             cfg.Metrics.Tagged(map[string]string{"endpoint": "csr"}),
		usshHostKeyCallback: cfg.UsshHostSigner,
		usshUserKeys:        cfg.Ussh,
		host:                cfg.Host,
		launchers:           cfg.Launchers,
	}
}

// ServeHTTP serves the CSR handler
func (h csrHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	var csr wonka.CertificateSigningRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&csr); err != nil {
		writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
		return
	}

	certToSign, err := wonka.UnmarshalCertificate(csr.Certificate)
	if err != nil {
		writeResponse(w, h, err, wonka.BadCertificateSigningRequest, http.StatusBadRequest)
		return
	}
	h.log = h.log.With(zap.Any("entity", certToSign.EntityName))

	h.log = h.log.With(zap.String("entity", certToSign.EntityName),
		zap.Int64("serial", int64(certToSign.Serial)),
		zap.String("hostname", certToSign.Host),
		zap.Any("tags", certToSign.Tags))

	if err := h.authCSR(ctx, csr, certToSign); err != nil {
		writeResponse(w, h, err, wonka.BadCertificateSigningRequest, http.StatusForbidden)
		return
	}

	h.log.Debug("verifying timestamps on request")
	now := time.Now()
	cTime := time.Unix(int64(certToSign.ValidAfter), 0)
	if !timehelper.WithinClockSkew(cTime, now, allowedClockSkew) {
		writeResponse(w, h, errTime, wonka.ErrTimeWindow, http.StatusBadRequest)
		return
	}

	// probably not a worthwhile test
	eTime := time.Unix(int64(certToSign.ValidBefore), 0)
	if now.Add(-allowedClockSkew).After(eTime) {
		writeResponse(w, h, errTime, wonka.CSRExpired, http.StatusBadRequest)
		return
	}

	if maxEtime := now.Add(maxCertificateTime); eTime.After(maxEtime) {
		h.log.Error("invalid certificate duration requested",
			zap.Any("old_etime", eTime),
			zap.Any("new_etime", maxEtime),
		)
		certToSign.ValidBefore = uint64(maxEtime.Unix())
	}

	h.log.Debug("signing certificate request")
	b, err := bytesForSigning(certToSign)
	if err != nil {
		writeResponse(w, h, err, wonka.BadCertificateSigningRequest, http.StatusBadRequest)
		return
	}

	sig, err := wonkacrypter.New().Sign(b, h.eccPrivateKey)
	if err != nil {
		writeResponse(w, h, err, wonka.BadCertificateSigningRequest, http.StatusBadRequest)
		return
	}

	sigHash := crypto.SHA256.New()
	sigHash.Write(sig)
	h.log.Debug("signature added",
		zap.Any("signature", base64.StdEncoding.EncodeToString(sigHash.Sum(nil))),
	)
	certToSign.Signature = sig

	certBytes, err := wonka.MarshalCertificate(*certToSign)
	if err != nil {
		h.log.Error("error marshalling reply certificate", zap.Error(err))
		return
	}
	csr.Certificate = certBytes

	writeResponse(w, h, nil, csr.Result, http.StatusOK, responseBody(csr))
}

func bytesForSigning(c *wonka.Certificate) ([]byte, error) {
	c2 := *c
	c2.Signature = nil
	return wonka.MarshalCertificate(c2)
}

func (h csrHandler) authCSR(ctx context.Context, csr wonka.CertificateSigningRequest, certToSign *wonka.Certificate) error {
	if csr.SigningCertificate != nil {
		signingCert, err := wonka.UnmarshalCertificate(csr.SigningCertificate)
		if err != nil {
			return errors.New(wonka.BadCertificateSigningRequest)
		}
		if wonka.IsCertGrantingCert(signingCert) {
			return h.cgCertVerify(csr, certToSign, signingCert)
		}
		return h.existingCertVerify(csr, certToSign, signingCert)
	}

	if csr.USSHCertificate != nil {
		_, err := h.usshVerify(csr, certToSign)
		return err
	}

	// this had better be a pre-enrolled service
	entity, err := h.db.Get(ctx, certToSign.EntityName)
	if err != nil {
		return fmt.Errorf("unknown entity %s. %v", certToSign.EntityName, err)
	}
	return h.verifyEnrolledCSR(csr, entity)
}

func (h csrHandler) verifyEnrolledCSR(csr wonka.CertificateSigningRequest, entity *wonka.Entity) error {
	toVerify := csr
	toVerify.Signature = nil
	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling csr to verify: %v", err)
	}

	pubKey, err := wonka.KeyFromCompressed(entity.ECCPublicKey)
	if err != nil {
		return fmt.Errorf("error parsing entity publickey: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerifyBytes, csr.Signature, pubKey); !ok {
		return errors.New("csr signature doesn't verify with pre-enrolled key")
	}

	return nil
}

// cgCertVerify verifies the signatures on a cert-granting cert
func (h csrHandler) cgCertVerify(csr wonka.CertificateSigningRequest, certToSign, signingCert *wonka.Certificate) error {
	// there are several steps to verifying a cert-granting cert. we need to verify the:
	// 1. csr was signed by the key in the signing certificate.
	// 2. valid ussh host cert at cert.Tags[TagUSSHcert].
	// 3. signing cert was signed by the ussh host key (private key).
	// 4. valid CertificateSignature at cert.Tags[TagLaunchRequest]
	// 5. cert-signature from 4. was signed by the embedded wonka certificate
	// 6. cert-signature from 4. contains a valid launch request
	//
	// Iff all of these signatures validate, and the timestamps are correct, then we
	// make sure things like the entityname on the cert matches the entityname from
	// the launch request, and the hostname from the ussh hostcert matches the hostname
	// from the launch request.

	// 1. verify this cert signed this csr
	if err := verifyCSRSignature(csr, signingCert); err != nil {
		return err
	}

	// 2.
	usshStr, ok := signingCert.Tags[wonka.TagUSSHCert]
	if !ok {
		return errors.New("no ussh host cert found")
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(usshStr))
	if err != nil {
		return fmt.Errorf("error parsing ssh authorized key: %v", err)
	}

	usshCert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return errors.New("non certificate provided")
	}

	if err := h.validUsshHostCert(usshCert, certToSign.Host); err != nil {
		return err
	}

	// 3.
	// TODO(pmoody): make this a helper function
	toVerify := *signingCert
	toVerify.Signature = nil
	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling certificate for signature verification: %v", err)
	}

	var sshSig ssh.Signature
	if err := ssh.Unmarshal(signingCert.Signature, &sshSig); err != nil {
		return fmt.Errorf("error unmarshalling ssh signature: %v", err)
	}

	if err := usshCert.Verify(toVerifyBytes, &sshSig); err != nil {
		return fmt.Errorf("error validating ussh signature on certificate: %v", err)
	}

	// verifyLaunchRequest does 4, 5 & 6
	lr, err := verifyLaunchRequest(signingCert.Tags[wonka.TagLaunchRequest])
	if err != nil {
		return err
	}

	// verify task-y stuff
	// Prevents stealing a launch request from hostYY and launching the task on hostXX.
	if !strings.EqualFold(lr.Hostname, certToSign.Host) ||
		!strings.EqualFold(lr.Hostname, usshCert.ValidPrincipals[0]) {
		return fmt.Errorf("request from invalid host. launch %q, cert %q, ussh %q",
			lr.Hostname, certToSign.Host, usshCert.ValidPrincipals[0])
	}

	if lr.SvcID != certToSign.EntityName {
		return fmt.Errorf("invalid entity name in request. launch %q, cert %q", lr.SvcID, certToSign.EntityName)
	}

	// we need to check check that the launch request adheres to the launchers.

	return nil
}

func (h csrHandler) existingCertVerify(csr wonka.CertificateSigningRequest, certToSign, signingCert *wonka.Certificate) error {
	if err := verifyCSRSignature(csr, signingCert); err != nil {
		return err
	}

	if certToSign.EntityName != signingCert.EntityName {
		return fmt.Errorf("invalid entity name on new cert, %q vs %q", signingCert.EntityName,
			certToSign.EntityName)
	}

	if certToSign.Host != signingCert.Host {
		return fmt.Errorf("invalid hostname on new cert (%q vs %q)", certToSign.Host, signingCert.Host)
	}

	if !reflect.DeepEqual(certToSign.Tags, signingCert.Tags) {
		h.log.With(
			zap.Any("old_tags", certToSign.Tags),
			zap.Any("new_tags", signingCert.Tags)).Warn("tags differ on signing cert")
		//return fmt.Errorf("invalid tags in new cert")
	}

	return nil
}

func verifyLaunchRequest(lrString string) (*wonka.LaunchRequest, error) {
	if lrString == "" {
		return nil, errors.New("no launch request included")
	}

	lrBytes, err := base64.StdEncoding.DecodeString(lrString)
	if err != nil {
		return nil, fmt.Errorf("error decoding signed launch request: %v", err)
	}

	var certSignature wonka.CertificateSignature
	if err := json.Unmarshal(lrBytes, &certSignature); err != nil {
		return nil, fmt.Errorf("error unmarshalling signed launch request: %v", err)
	}

	if err := wonka.VerifyCertificateSignature(certSignature); err != nil {
		return nil, fmt.Errorf("error verifying launch request: %v", err)
	}

	var lr wonka.LaunchRequest
	if err := json.Unmarshal(certSignature.Data, &lr); err != nil {
		return nil, fmt.Errorf("error extracting launch requesting from signature: %v", err)
	}

	return &lr, nil
}

func verifyCSRSignature(csr wonka.CertificateSigningRequest, signingCert *wonka.Certificate) error {
	toVerify := csr
	toVerify.Signature = nil
	toVerify.SignatureType = ""
	toVerify.Result = ""
	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling bytes to verify: %v", err)
	}

	if ok := signingCert.Verify(toVerifyBytes, csr.Signature); !ok {
		return errors.New("signature on csr doesn't validate")
	}
	return nil
}

func (h csrHandler) usshVerify(csr wonka.CertificateSigningRequest, certToSign *wonka.Certificate) (*ssh.Certificate, error) {
	// verify the signature on the request
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(csr.USSHCertificate)
	if err != nil {
		return nil, fmt.Errorf("error parsing ssh authorized key: %v", err)
	}

	usshCert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, errors.New("non certificate provided")
	}

	switch usshCert.CertType {
	case ssh.HostCert:
		return usshCert, h.usshHostVerify(csr, usshCert, certToSign)
	case ssh.UserCert:
		return usshCert, h.usshUserVerify(usshCert, certToSign.EntityName)
	}

	return nil, errors.New("invalid certificate")
}

func (h csrHandler) usshUserVerify(usshCert *ssh.Certificate, principal string) error {
	name := strings.Split(principal, "@")
	if len(name) != 2 {
		return fmt.Errorf("invalid entity name %v", principal)
	}

	return wonkassh.CheckUserCert(name[0], usshCert, h.usshUserKeys)
}

func (h csrHandler) validUsshHostCert(usshCert *ssh.Certificate, host string) error {
	// only add the uber domain suffix if the host doesn't look like a domain already, e.g. has a domain separator (`.`) character
	if !strings.Contains(host, ".") {
		host += _uberHostSuffix
	}
	name := fmt.Sprintf("%s:22", host)
	addr := &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}
	if err := h.usshHostKeyCallback(name, addr, usshCert); err != nil {
		return fmt.Errorf("error validating signing ussh certificate: %v", err)
	}

	return nil
}

func (h csrHandler) usshHostVerify(csr wonka.CertificateSigningRequest, usshCert *ssh.Certificate, certToSign *wonka.Certificate) error {
	if err := h.validUsshHostCert(usshCert, certToSign.Host); err != nil {
		return fmt.Errorf("ussh validation failed: %v", err)
	}

	// T1425115: We do not accept a USSH host certificate for validation of a user entity.
	switch certToSign.Type {
	case wonka.EntityTypeService:
	case wonka.EntityTypeHost:
	case wonka.EntityTypeInvalid:
		// BUG(T1426625)
		// We need to accept invalid type certs because wonkad does not correctly populate this field.
	default:
		return fmt.Errorf("cannot validate a wonka %q cert with a USSH host cert", certToSign.Type)
	}

	// T1425115: Since we only allow service and host validation here, we also check for obviously erroneous entity names
	if name := certToSign.EntityName; isPersonnelClaim(name) || isADGroupClaim(name) {
		return fmt.Errorf("invalid entity name for USSH host cert validation: %q", certToSign.EntityName)
	}

	// TODO(pmoody): when D1234985 lands (and is rolled out) start checking for
	// cert.EntityType == EntityTypeHost here too.
	// verify the launch request
	if csr.LaunchRequest != nil {
		lrStr := base64.StdEncoding.EncodeToString(csr.LaunchRequest)
		lr, err := verifyLaunchRequest(lrStr)
		if err != nil {
			return err
		}

		if lr.Hostname != usshCert.ValidPrincipals[0] {
			return fmt.Errorf("launch request host %q doesn't match ussh host %q",
				lr.Hostname, usshCert.ValidPrincipals[0])
		}
	}

	h.log.Warn("no launch request included")
	return nil
}
