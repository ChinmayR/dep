package wonka

import (
	"bytes"
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
	"golang.org/x/crypto/ssh/agent"
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

// CertEntityType sets the entity type for this cert
func CertEntityType(e EntityType) CertificateOption {
	return func(c *Certificate) {
		c.Type = e
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

// CertUSSHCertTag adds a ussh certificate to the tags.
func CertUSSHCertTag(t string) CertificateOption {
	return func(c *Certificate) {
		c.Tags[TagUSSHCert] = t
	}
}

// CertLaunchRequestTag adds a luanch request to the tags.
func CertLaunchRequestTag(t string) CertificateOption {
	return func(c *Certificate) {
		c.Tags[TagLaunchRequest] = t
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
// validity period defined by the certificate, taking into account
// a default clock slew.
func (c *Certificate) CheckCertificate() error {
	if err := c.ValidateSignature(); err != nil {
		return err
	}

	now := time.Now()
	if c.NotYetValid(now.Add(clockSkew)) {
		return errors.New("certificate is not yet valid")
	}

	if c.Expired(now.Add(-clockSkew)) {
		return errors.New("certificate expired")
	}

	return nil
}

// Expired returns true if the certificate has expired relative to now.
func (c *Certificate) Expired(now time.Time) bool {
	expireTime := time.Unix(int64(c.ValidBefore), 0)
	return now.After(expireTime)
}

// NotYetValid returns true if the cerficate is not yet valid relative to now.
func (c *Certificate) NotYetValid(now time.Time) bool {
	createTime := time.Unix(int64(c.ValidAfter), 0)
	return now.Before(createTime)
}

// ValidateSignature checks that the certificate was
// signed by the wonkamaster. It performs no additional
// validation.
func (c *Certificate) ValidateSignature() error {
	certToVerify := *c
	certToVerify.Signature = nil

	toVerify, err := json.Marshal(certToVerify)
	if err != nil {
		return fmt.Errorf("error marshalling certificate to check: %v", err)
	}

	if ok := wonkacrypter.VerifyAny(toVerify, c.Signature, WonkaMasterPublicKeys); !ok {
		return errors.New("wonkacert signature verification failure")
	}

	return nil
}

// Verify verifies that the given data was signed by the private key associated
// with this certificate.
func (c *Certificate) Verify(data, sig []byte) bool {
	pubKey, err := c.PublicKey()
	if err != nil {
		return false
	}

	return wonkacrypter.New().Verify(data, sig, pubKey)
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

// equal tests if two certificates are equal without the overhead of
// reflect.DeepEqual.
func (c *Certificate) equal(rhs *Certificate) bool {
	if c == rhs {
		return true
	}

	// Evalute deep equality
	eq := c.EntityName == rhs.EntityName &&
		c.Host == rhs.Host &&
		bytes.Equal(c.Key, rhs.Key) &&
		c.Serial == rhs.Serial &&
		bytes.Equal(c.Signature, rhs.Signature) &&
		c.Type == rhs.Type &&
		c.ValidAfter == rhs.ValidAfter &&
		c.ValidBefore == rhs.ValidBefore

	if !eq {
		return false
	}

	// Finally test the tags
	if len(c.Tags) != len(rhs.Tags) {
		return false
	}

	for k, v := range c.Tags {
		if v2, ok := rhs.Tags[k]; !ok || (ok && v != v2) {
			return false
		}
	}

	return true
}

func signCSRWithSSH(c *Certificate,
	req *CertificateSignature,
	sshAgent agent.Agent,
	log *zap.Logger) (*CertificateSigningRequest, error) {

	keys, err := sshAgent.List()
	if err != nil {
		return nil, fmt.Errorf("error getting valid signers from ssh-agent: %v", err)
	}

	log.Debug("number of signers found", zap.Int("num", len(keys)))
	// TODO(pmoody): validate that this is a ussh certificate
	var usshCert *ssh.Certificate
	for _, k := range keys {
		k, err := ssh.ParsePublicKey(k.Blob)
		if err != nil {
			log.Error("error parsing ssh key", zap.Error(err))
			continue
		}
		c, ok := k.(*ssh.Certificate)
		if !ok {
			log.Info("not a ussh certificate")
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

	sig, err := sshAgent.Sign(usshCert, toSign)
	if err != nil {
		return nil, fmt.Errorf("error signing csr: %v", err)
	}

	csr.Signature = sig.Blob
	csr.SignatureType = sig.Format

	return csr, nil
}

func signCSRWithCert(cert *Certificate, signingCert *Certificate, signingKey *ecdsa.PrivateKey) (*CertificateSigningRequest, error) {
	if cert == nil {
		return nil, errors.New("certificate is nil")
	}

	if signingCert == nil || signingKey == nil {
		return nil, fmt.Errorf("nil cert %v and/or nil key %v", signingCert == nil, signingKey == nil)
	}

	certBytes, err := MarshalCertificate(*cert)
	if err != nil {
		return nil, fmt.Errorf("error marshalling signing certificate for csr: %v", err)
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

func (w *uberWonka) signCSRWithEnrolled(cert *Certificate) (*CertificateSigningRequest, error) {
	if cert == nil {
		return nil, errors.New("certificate is nil")
	}

	certBytes, err := MarshalCertificate(*cert)
	if err != nil {
		return nil, fmt.Errorf("error marshalling new certificate for csr: %v", err)
	}

	csr := &CertificateSigningRequest{
		Certificate: certBytes,
	}

	toSign, err := json.Marshal(csr)
	if err != nil {
		return nil, fmt.Errorf("error marshalling csr for signing: %v", err)
	}

	csr.Signature, err = wonkacrypter.New().Sign(toSign, w.readECCKey())
	if err != nil {
		return nil, fmt.Errorf("error signing csr: %v", err)
	}

	return csr, nil
}

// signCertificate signs c using the signing certificate and signingKey.
func signCertificate(ctx context.Context,
	c, signingCert *Certificate,
	signingKey *ecdsa.PrivateKey,
	h *httpRequester) error {

	if c == nil {
		return errors.New("certificate is nil")
	}

	if c.EntityName == "" {
		return errors.New("no entity name provided")
	}

	if c.Host == "" {
		return errors.New("no hostname provided")
	}

	if signingCert == nil {
		return errors.New("cannot refresh cert without existing valid cert")
	}

	if h == nil {
		return errors.New("httpRequester is nil")
	}

	var csr *CertificateSigningRequest
	var err error

	// For refreshing a certificate we only ever need to sign with existing certificate.
	csr, err = signCSRWithCert(c, signingCert, signingKey)
	if err != nil {
		return fmt.Errorf("error signing request: %v", err)
	}

	var reply CertificateSigningRequest
	if err := h.Do(ctx, csrEndpoint, csr, &reply); err != nil {
		return fmt.Errorf("error getting cert signed by wonkamaster: %v", err)
	}

	replyCert, err := UnmarshalCertificate(reply.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	*c = *replyCert

	return c.CheckCertificate()
}

// CertificateSignRequest tries to get wonkamaster to sign the given certificate.
// On success, nil is returned and the certificate passed in has the Signature field
// updated. On error, the passed in certificate is un-modified and an error is returned.
func (w *uberWonka) CertificateSignRequest(ctx context.Context,
	c *Certificate,
	req *CertificateSignature) error {

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

	existingCert := w.readCertificate()
	// we should probably only sign csr's with the ssh-agent if there's a launch request included.
	if req != nil || w.sshAgent != nil {
		csr, err = signCSRWithSSH(c, req, w.sshAgent, w.log)
	} else if existingCert != nil {
		csr, err = signCSRWithCert(c, existingCert, w.readECCKey())
	} else {
		// pre-enrolled
		csr, err = w.signCSRWithEnrolled(c)
	}

	if err != nil {
		return fmt.Errorf("error signing request: %v", err)
	}

	var reply CertificateSigningRequest
	if err := w.httpRequester.Do(ctx, csrEndpoint, csr, &reply); err != nil {
		return fmt.Errorf("error getting cert signed by wonkamaster: %v", err)
	}

	replyCert, err := UnmarshalCertificate(reply.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	*c = *replyCert

	return c.CheckCertificate()
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

func refreshCertFromWonkamaster(ctx context.Context,
	signingCert *Certificate,
	signingKey *ecdsa.PrivateKey,
	h *httpRequester) (*Certificate, *ecdsa.PrivateKey, error) {

	if signingCert == nil || signingKey == nil {
		return nil, nil, fmt.Errorf("nil cert %v and/or nil key %v", signingCert == nil,
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
		return nil, nil, fmt.Errorf("error generating new certificate: %v", err)
	}

	// This method runs as a background refresh job in a go routine.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := signCertificate(ctx, cert, signingCert, signingKey, h); err != nil {
		return nil, nil, fmt.Errorf("error refreshing certificate: %v", err)
	}

	return cert, key, err
}

// IsCertGrantingCert returns tre if this certificate can only be used to
// request a fully-signed wonka certificate.
func IsCertGrantingCert(cert *Certificate) bool {
	if cert == nil {
		return false
	}

	_, usshOk := cert.Tags[TagUSSHCert]
	_, launchReqOk := cert.Tags[TagLaunchRequest]
	return usshOk && launchReqOk
}

func refreshCertFromWonkad(cert *Certificate, key *ecdsa.PrivateKey) (*Certificate, *ecdsa.PrivateKey, error) {
	if cert == nil || key == nil {
		return nil, nil, fmt.Errorf("nil cert %v and/or nil key %v", cert == nil, key == nil)
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
		return nil, nil, fmt.Errorf("error marshalling msg to sign: %v", err)
	}

	sig, err := wonkacrypter.New().Sign(toSign, key)
	if err != nil {
		return nil, nil, fmt.Errorf("error signing message: %v", err)
	}

	req.Signature = []byte(base64.StdEncoding.EncodeToString(sig))
	toWrite, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshalling msg to send to wonkad: %v", err)
	}

	conn, err := net.Dial("tcp", WonkadTCPAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to wonkad: %v", err)
	}

	if _, err := conn.Write(toWrite); err != nil {
		return nil, nil, fmt.Errorf("error writing request to wonkad: %v", err)
	}

	b, err := ioutil.ReadAll(conn)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading reply from wonkad: %v", err)
	}

	var repl WonkadReply
	if err := json.Unmarshal(b, &repl); err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling reply from wonkad: %v", err)
	}

	newCert, err := UnmarshalCertificate(repl.Certificate)
	if err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	newKey, err := x509.ParseECPrivateKey(repl.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing ec private key: %v", err)
	}

	return newCert, newKey, nil
}

// upgradeCGCert turns a cert-granting cert into a regular wonka cert.
// we hold the lock all the way through this function to avoid any improable
// race conditions where we're called twice.
// this also means that we can't use any lock-grabbing helpers, and nothing that
// we call can use any lock grabbing helpers.
func (w *uberWonka) upgradeCGCert(ctx context.Context) error {
	w.clientKeysMu.Lock()
	defer w.clientKeysMu.Unlock()

	if !IsCertGrantingCert(w.certificate) {
		return nil
	}

	lr, err := unmarshalLaunchRequest(w.certificate.Tags[TagLaunchRequest])
	if err != nil {
		return err
	}

	taskID := lr.TaskID
	if taskID == "" {
		taskID = lr.InstID
	}

	cert, privKey, err := NewCertificate(CertHostname(lr.Hostname),
		CertEntityName(lr.SvcID), CertTaskIDTag(taskID))
	if err != nil {
		return err
	}

	// we do this manually because we can't call CertificateSignRequest
	// due to the lock requirements.
	csr, err := signCSRWithCert(cert, w.certificate, w.clientECC)
	if err != nil {
		return fmt.Errorf("error signing upgrade cert: %v", err)
	}

	var reply CertificateSigningRequest
	if err := w.httpRequester.Do(ctx, csrEndpoint, csr, &reply); err != nil {
		return fmt.Errorf("error getting cert signed by wonkamaster: %v", err)
	}

	cert, err = UnmarshalCertificate(reply.Certificate)
	if err != nil {
		return fmt.Errorf("error unmarshalling certificate: %v", err)
	}

	if err := cert.CheckCertificate(); err != nil {
		return fmt.Errorf("certificate is invalid: %v", err)
	}

	w.certificate = cert
	w.clientECC = privKey

	return nil
}

func unmarshalLaunchRequest(req string) (*LaunchRequest, error) {
	if req == "" {
		return nil, errors.New("no launch request supplied")
	}

	reqBytes, err := base64.StdEncoding.DecodeString(req)
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding launch request: %v", err)
	}

	var sig CertificateSignature
	if err := json.Unmarshal(reqBytes, &sig); err != nil {
		return nil, fmt.Errorf("error unmarshalling signature: %v", err)
	}

	var lr LaunchRequest
	if err := json.Unmarshal(sig.Data, &lr); err != nil {
		return nil, fmt.Errorf("error json unmarshalling launch request: %v", err)
	}

	return &lr, nil
}
