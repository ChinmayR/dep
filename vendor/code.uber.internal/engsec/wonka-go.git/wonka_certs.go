package wonka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"net"
	"os"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// MarshalCertificate turns a Certificate into wire-format
func MarshalCertificate(c Certificate) ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshalling certificate: %v", err)
	}
	return b, nil
}

// UnmarshalCertificate turns a wire-format certificate into
// its constituent struct.
func UnmarshalCertificate(d []byte) (*Certificate, error) {
	c := &Certificate{}
	err := json.Unmarshal(d, c)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling certificate: %v", err)
	}
	return c, nil
}

// CertificateOption implements the option pattern for NewCertificate
type CertificateOption func(*Certificate)

// CertHostname adds this hostname to the certificate. If the hostname is not set
// wonkamaster will refuse to sign the certificate.
func CertHostname(h string) CertificateOption {
	return func(c *Certificate) {
		c.Host = h
	}
}

// CertEntityName adds the given entity, or service name, to the certificate.
// If the entity name is not set, wonkamaster will refuse to sign the certificate.
func CertEntityName(s string) CertificateOption {
	return func(c *Certificate) {
		c.EntityName = s
	}
}

// CertTaskIDTag sets the task id tag.
func CertTaskIDTag(t string) CertificateOption {
	return func(c *Certificate) {
		c.Tags[TagTaskID] = t
	}
}

// CertRuntimeTag adds the current runtime environment to the certificate.
func CertRuntimeTag(t string) CertificateOption {
	return func(c *Certificate) {
		c.Tags[TagRuntime] = t
	}
}

// NewCertificate returns a certificate and private key for the given entity/hostname
func NewCertificate(opts ...CertificateOption) (*Certificate, *ecdsa.PrivateKey, error) {
	privkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating wonka cert key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, nil, fmt.Errorf("error generating serial number: %v", err)
	}

	pubkey, err := x509.MarshalPKIXPublicKey(&privkey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshalling public key: %v", err)
	}

	cert := &Certificate{
		Key:         pubkey,
		ValidAfter:  uint64(time.Now().Unix()),
		ValidBefore: uint64(time.Now().Add(20 * time.Hour).Unix()),
		Serial:      serial.Uint64(),
		Tags:        make(map[string]string),
	}

	for _, opt := range opts {
		opt(cert)
	}

	return cert, privkey, nil
}

// NewCertificateSignature signs the given data and certificate with the given private key.
// The certificate is included with the signature so it can be verified offline. The certificate
// itself chains back to wonkamaster.
func NewCertificateSignature(cert Certificate, key *ecdsa.PrivateKey, toSign []byte) (*CertificateSignature, error) {
	pubKey, err := cert.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("error reading public key from certificate: %v", err)
	}

	if !pubKeysEq(pubKey, &key.PublicKey) {
		return nil, errors.New("private key does not match certificate")
	}

	certSignature := CertificateSignature{
		Certificate: cert,
		Timestamp:   time.Now().Unix(),
		Data:        []byte(toSign),
	}

	certSigToSign, err := json.Marshal(certSignature)
	if err != nil {
		return nil, fmt.Errorf("error marshalling certificate signature: %v", err)
	}

	certSignature.Signature, err = wonkacrypter.New().Sign(certSigToSign, key)
	if err != nil {
		return nil, fmt.Errorf("error signing certificate: %v", err)
	}

	return &certSignature, nil
}

// VerifyCertificateSignature verifies that a given signature is valid.
// A CertificateSignature includes the certificate associated with the private
// key which was used to generate the signature. The certificate is signed by
// wonkamaster.
// The signature over the data is verified, and then the signature in the certificate
// itself is verified.
func VerifyCertificateSignature(certSignature CertificateSignature) error {
	pubKey, err := certSignature.Certificate.PublicKey()
	if err != nil {
		return fmt.Errorf("error parsing publickey from signing certificate: %v", err)
	}

	sig := certSignature.Signature
	certSignature.Signature = nil

	data, err := json.Marshal(certSignature)
	if err != nil {
		return fmt.Errorf("error re-marshalling signature")
	}

	if ok := wonkacrypter.New().Verify(data, sig, pubKey); !ok {
		return errors.New("signature doesn't match")
	}

	// docker doesn't re-create launch requests when a given task on a host needs
	// to be restarted. IOW, a task might persist on a given host for months with
	// the 'same' launch request. Since CheckCertificate() checks the validity of
	// the certificate *now*, we'll need a modified version for docker that will
	// check the validity when the task was intially launched.
	return certSignature.Certificate.CheckCertificate()
}

// PublicKey returns the public key associated with the certificate.
func (c *Certificate) PublicKey() (*ecdsa.PublicKey, error) {
	pub, err := x509.ParsePKIXPublicKey(c.Key)
	if err != nil {
		return nil, fmt.Errorf("error parsing key: %v", err)
	}

	k, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("not an ecdsa public key")
	}

	return k, nil
}

// CheckCertificate validates a certificate. It checks that it was
// signed by the wonkamaster and the current time falls inside the
// validity period.
func (c *Certificate) CheckCertificate() error {
	certToVerify := *c
	certToVerify.Signature = nil

	toVerify, err := json.Marshal(certToVerify)
	if err != nil {
		return fmt.Errorf("error marshalling certificate to check: %v", err)
	}

	if ok := wonkacrypter.VerifyAny(toVerify, c.Signature, WonkaMasterPublicKeys); !ok {
		return errors.New("wonkacert signature verification failure")
	}

	now := time.Now()
	createTime := time.Unix(int64(c.ValidAfter), 0)
	if now.Add(clockSkew).Before(createTime) {
		return errors.New("certificate is not yet valid")
	}

	expireTime := time.Unix(int64(c.ValidBefore), 0)
	if now.Add(-clockSkew).After(expireTime) {
		return errors.New("certificate expired")
	}

	return nil
}

// SignCertificate signs a wonka certificate with the given private key.
func (c *Certificate) SignCertificate(signer *ecdsa.PrivateKey) error {
	toSignCert := *c
	toSignCert.Signature = nil
	toSign, err := json.Marshal(toSignCert)
	if err != nil {
		return fmt.Errorf("error marshalling certificate to sign: %v", err)
	}

	sig, err := wonkacrypter.New().Sign(toSign, signer)
	if err != nil {
		return fmt.Errorf("error signing certificate: %v", err)
	}

	c.Signature = sig
	return nil
}

func (w *uberWonka) signCSRWithSSH(c *Certificate, req *CertificateSignature) (*CertificateSigningRequest, error) {
	keys, err := w.sshAgent.List()
	if err != nil {
		return nil, fmt.Errorf("error getting valid signers from ssh-agent: %v", err)
	}

	w.log.Debug("number of signers found", zap.Int("num", len(keys)))
	// TODO(pmoody): validate that this is a ussh certificate
	var usshCert *ssh.Certificate
	for _, k := range keys {
		k, err := ssh.ParsePublicKey(k.Blob)
		if err != nil {
			w.log.Error("error parsing ssh key", zap.Error(err))
			continue
		}
		c, ok := k.(*ssh.Certificate)
		if !ok {
			w.log.Info("not a ussh certificate")
			continue
		}
		usshCert = c
		break
	}

	if usshCert == nil {
		return nil, errors.New("no ussh certs found")
	}

	reqBytes := []byte{}
	if req != nil {
		reqBytes, err = json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("error marshalling launch request: %v", err)
		}
	}

	cert, err := MarshalCertificate(*c)
	if err != nil {
		return nil, err
	}

	csr := &CertificateSigningRequest{
		Certificate:     cert,
		USSHCertificate: ssh.MarshalAuthorizedKey(usshCert),
		LaunchRequest:   reqBytes,
	}

	toSign, err := json.Marshal(csr)
	if err != nil {
		return nil, fmt.Errorf("error marshalling csr to sign: %v", err)
	}

	sig, err := w.sshAgent.Sign(usshCert, toSign)
	if err != nil {
		return nil, fmt.Errorf("error signing csr: %v", err)
	}

	csr.Signature = sig.Blob
	csr.SignatureType = sig.Format

	return csr, nil
}

func (w *uberWonka) signCSRWithCert(cert *Certificate) (*CertificateSigningRequest, error) {
	if cert == nil {
		return nil, errors.New("certificate is nil")
	}

	signingCert := w.readCertificate()
	signingKey := w.readECCKey()
	if signingCert == nil || signingKey == nil {
		return nil, fmt.Errorf("nil cert %v and/or nil key %v", signingCert == nil, signingKey == nil)
	}

	certBytes, err := MarshalCertificate(*cert)
	if err != nil {
		return nil, fmt.Errorf("error marshalling new certificate for csr: %v", err)
	}

	signingCertBytes, err := MarshalCertificate(*signingCert)
	if err != nil {
		return nil, fmt.Errorf("error marshalling certificate for csr: %v", err)
	}

	csr := &CertificateSigningRequest{
		Certificate:        certBytes,
		SigningCertificate: signingCertBytes,
	}

	toSign, err := json.Marshal(csr)
	if err != nil {
		return nil, fmt.Errorf("error marshalling csr for signing: %v", err)
	}

	sig, err := wonkacrypter.New().Sign(toSign, signingKey)
	if err != nil {
		return nil, fmt.Errorf("error signing csr: %v", err)
	}
	csr.Signature = sig

	return csr, nil
}

// CertificateSignRequest tries to get wonkamaster to sign the given certificate.
// On success, nil is returned and the certificate passed in has the Signature field
// updated. On error, the passed in certificate is un-modified and an error is returned.
func (w *uberWonka) CertificateSignRequest(ctx context.Context, c *Certificate, req *CertificateSignature) error {
	if c == nil {
		return errors.New("certificate is nil")
	}

	if c.EntityName == "" {
		return errors.New("no entity name provided")
	}

	if c.Host == "" {
		return errors.New("no hostname provided")
	}

	var csr *CertificateSigningRequest
	var err error

	// we should probably only sign csr's with the ssh-agent if there's a launch request included.
	if req != nil || w.sshAgent != nil {
		csr, err = w.signCSRWithSSH(c, req)
	} else {
		csr, err = w.signCSRWithCert(c)
	}

	if err != nil {
		return fmt.Errorf("error signing request: %v", err)
	}

	var reply CertificateSigningRequest
	if err := w.httpRequest(ctx, csrEndpoint, csr, &reply); err != nil {
		return fmt.Errorf("error getting cert signed by wonkamaster: %v", err)
	}

	replyCert, err := UnmarshalCertificate(reply.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	*c = *replyCert

	return c.CheckCertificate()
}

func (w *uberWonka) refreshWonkaCert(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// every 30 minutes, try to refresh the cert by asking wonkamster first, and
			// wonkad on localhost second.
			if err := w.refreshCertFromWonkamaster(ctx); err != nil {
				w.log.Warn("error refreshing from wonkamaster", zap.Error(err))
				if err := w.refreshCertFromWonkad(); err != nil {
					w.log.Warn("error refreshing from wonkad", zap.Error(err))
				}
			}
		}
	}
}

// TODO(pmoody): extract the csr generation to a helper
func (w *uberWonka) refreshCertFromWonkamaster(ctx context.Context) error {
	signingCert := w.readCertificate()
	signingKey := w.readECCKey()
	if signingCert == nil || signingKey == nil {
		return fmt.Errorf("nil cert %v and/or nil key %v", signingCert == nil,
			signingKey == nil)
	}

	runtimeEnv, ok := signingCert.Tags[TagRuntime]
	if !ok {
		runtimeEnv = os.Getenv("UBER_RUNTIME_ENVIRONMENT")
	}
	cert, key, err := NewCertificate(
		CertEntityName(signingCert.EntityName),
		CertHostname(signingCert.Host),
		CertTaskIDTag(signingCert.Tags[TagTaskID]),
		CertRuntimeTag(runtimeEnv))
	if err != nil {
		return fmt.Errorf("error generating new certificate: %v", err)
	}

	// This method runs as a background refresh job in a go routine.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := w.CertificateSignRequest(ctx, cert, nil); err != nil {
		return fmt.Errorf("error refreshing certificate: %v", err)
	}

	w.writeCertAndKey(cert, key)
	return nil
}

func (w *uberWonka) refreshCertFromWonkad() error {
	key := w.readECCKey()
	cert := w.readCertificate()
	if cert == nil || key == nil {
		return fmt.Errorf("nil cert %v and/or nil key %v", cert == nil, key == nil)
	}

	taskID, ok := cert.Tags["TaskID"]
	if !ok {
		taskID = os.Getenv("MESOS_EXECUTOR_ID")
		if taskID == "" {
			taskID = os.Getenv("UDEPLOY_INSTANCE_NAME")
		}
	}

	// this is going to fail until mesos/docker figure out how they're going to be giving us the task id.
	req := WonkadRequest{
		Service:     cert.EntityName,
		TaskID:      taskID,
		Certificate: *cert,
	}

	toSign, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshalling msg to sign: %v", err)
	}

	sig, err := wonkacrypter.New().Sign(toSign, key)
	if err != nil {
		return fmt.Errorf("error signing message: %v", err)
	}

	req.Signature = []byte(base64.StdEncoding.EncodeToString(sig))
	toWrite, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshalling msg to send to wonkad: %v", err)
	}

	conn, err := net.Dial("tcp", WonkadTCPAddress)
	if err != nil {
		return fmt.Errorf("error connecting to wonkad: %v", err)
	}

	if _, err := conn.Write(toWrite); err != nil {
		return fmt.Errorf("error writing request to wonkad: %v", err)
	}

	b, err := ioutil.ReadAll(conn)
	if err != nil {
		return fmt.Errorf("error reading reply from wonkad: %v", err)
	}

	var repl WonkadReply
	if err := json.Unmarshal(b, &repl); err != nil {
		return fmt.Errorf("error unmarshalling reply from wonkad: %v", err)
	}

	cert, err = UnmarshalCertificate(repl.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	key, err = x509.ParseECPrivateKey(repl.PrivateKey)
	if err != nil {
		return fmt.Errorf("error parsing ec private key: %v", err)
	}

	w.writeCertAndKey(cert, key)
	return nil
}

// ValidCertFromBytes returns the certificate unmarshalled certificate if it's good.
func ValidCertFromBytes(b []byte) (*Certificate, error) {
	cert, err := UnmarshalCertificate(b)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling cert reply: %v", err)
	}

	if err := cert.CheckCertificate(); err != nil {
		return nil, fmt.Errorf("new certificate is invalid: %v", err)
	}

	return cert, nil
}
