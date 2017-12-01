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
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/timehelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"

	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

var (
	allowedClockSkew   = time.Minute
	maxCertificateTime = 20 * time.Hour
)

type csrHandler struct {
	eccPrivateKey       *ecdsa.PrivateKey
	log                 *zap.Logger
	metrics             tally.Scope
	usshHostKeyCallback ssh.HostKeyCallback
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
		log:                 cfg.Logger.With(zap.String("endpoint", "csr")),
		metrics:             cfg.Metrics.Tagged(map[string]string{"endpoint": "csr"}),
		usshHostKeyCallback: cfg.UsshHostSigner,
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

	cert, err := wonka.UnmarshalCertificate(csr.Certificate)
	if err != nil {
		writeResponse(w, h, err, wonka.BadCertificateSigningRequest, http.StatusBadRequest)
		return
	}

	h.log = h.log.With(zap.Any("entity", cert.EntityName))
	if err := h.authCSR(csr, cert); err != nil {
		writeResponse(w, h, err, wonka.BadCertificateSigningRequest, http.StatusForbidden)
		return
	}

	h.log.Debug("verifying timestamps on request")
	now := time.Now()
	cTime := time.Unix(int64(cert.ValidAfter), 0)
	if !timehelper.WithinClockSkew(cTime, now, allowedClockSkew) {
		writeResponse(w, h, errTime, wonka.ErrTimeWindow, http.StatusBadRequest)
		return
	}

	// probably not a worthwhile test
	eTime := time.Unix(int64(cert.ValidBefore), 0)
	if now.Add(-allowedClockSkew).After(eTime) {
		writeResponse(w, h, errTime, wonka.CSRExpired, http.StatusBadRequest)
		return
	}

	if maxEtime := now.Add(maxCertificateTime); eTime.After(maxEtime) {
		h.log.Error("invalid certificate duration requested",
			zap.Any("old_etime", eTime),
			zap.Any("new_etime", maxEtime),
		)

		cert.ValidBefore = uint64(maxEtime.Unix())
	}

	h.log.Debug("signing certificate request")
	b, err := bytesForSigning(cert)
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
	cert.Signature = sig

	certBytes, err := wonka.MarshalCertificate(*cert)
	if err != nil {
		h.log.Error("error marshalling reply certificate", zap.Error(err))
		return
	}
	csr.Certificate = certBytes

	h.log = h.log.With(zap.Any("entity", cert.EntityName),
		zap.Any("serial", cert.Serial),
		zap.Any("hostname", cert.Host),
		zap.Any("tags", cert.Tags))

	writeResponse(w, h, nil, csr.Result, http.StatusOK, responseBody(csr))
}

func bytesForSigning(c *wonka.Certificate) ([]byte, error) {
	c2 := *c
	c2.Signature = nil
	return wonka.MarshalCertificate(c2)
}

func (h csrHandler) authCSR(csr wonka.CertificateSigningRequest, cert *wonka.Certificate) error {
	if len(csr.LaunchRequest) != 0 {
		// not yet implemented, but, if there's a launch request, or mission statement,
		// we validate that the certificate is for something this particular launcher is allowed
		// to request
		return nil
	} else if csr.SigningCertificate != nil {
		h.log.Debug("verifying csr from an existing certificate")
		// handle a csr from a an existing service
		if err := h.existingCertVerify(csr); err != nil {
			return fmt.Errorf("error validating existing certificate: %v", err)
		}
		return nil
	} else if csr.USSHCertificate != nil {
		h.log = h.log.With(zap.Any("host", cert.Host))
		h.log.Debug("verifying ussh signature")
		if err := h.usshHostVerify(csr, cert); err != nil {
			return fmt.Errorf("error validating host ussh cert: %v", err)
		}
		return nil
	}

	return errors.New("unsigned request")
}

func (h csrHandler) existingCertVerify(csr wonka.CertificateSigningRequest) error {
	signingCert, err := wonka.ValidCertFromBytes(csr.SigningCertificate)
	if err != nil {
		return fmt.Errorf("invalid signing certificate: %v", err)
	}

	pubKey, err := signingCert.PublicKey()
	if err != nil {
		return fmt.Errorf("error getting signing cert public key: %v", err)
	}

	toVerify := csr
	toVerify.Signature = nil

	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling bytes to verify: %v", err)
	}

	certToSign, err := wonka.UnmarshalCertificate(csr.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	if certToSign.EntityName != signingCert.EntityName {
		return fmt.Errorf("invalid entity name on new cert")
	}

	if certToSign.Host != signingCert.Host {
		return fmt.Errorf("invalid hostname on new cert")
	}

	if !reflect.DeepEqual(certToSign.Tags, signingCert.Tags) {
		h.log.With(
			zap.Any("old_tags", certToSign.Tags),
			zap.Any("new_tags", signingCert.Tags)).Warn("tags differ on signing cert")
		//return fmt.Errorf("invalid tags in new cert")
	}

	if ok := wonkacrypter.New().Verify(toVerifyBytes, csr.Signature, pubKey); !ok {
		return fmt.Errorf("csr signature doesn't verify")
	}

	return nil
}

func (h csrHandler) usshHostVerify(csr wonka.CertificateSigningRequest, cert *wonka.Certificate) error {
	// verify the launch request
	if len(csr.LaunchRequest) != 0 {
		var certSignature wonka.CertificateSignature
		if err := json.Unmarshal(csr.LaunchRequest, &certSignature); err != nil {
			return fmt.Errorf("error unmarshalling launch request: %v", err)
		}

		if err := wonka.VerifyCertificateSignature(certSignature); err != nil {
			return fmt.Errorf("error verifying launch request: %v", err)
		}
	} else {
		h.log.Warn("no launch request included")
	}

	// verify the signature on the request
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(csr.USSHCertificate))
	if err != nil {
		return fmt.Errorf("error parsing ssh authorized key: %v", err)
	}

	signingCert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return errors.New("non certificate provided")
	}

	if cert.Host != signingCert.ValidPrincipals[0] {
		return errors.New("invalid hostname on signing cert")
	}

	h.log = h.log.With(zap.Int64("ussh_serial", int64(cert.Serial)),
		zap.String("authtype", "USSHHostCert"))

	name := fmt.Sprintf("%s:22", cert.Host)
	addr := &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}
	if err := h.usshHostKeyCallback(name, addr, signingCert); err != nil {
		return fmt.Errorf("error validating signing certificate: %v", err)
	}

	return nil
}
