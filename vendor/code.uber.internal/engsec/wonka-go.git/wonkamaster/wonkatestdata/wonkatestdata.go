package wonkatestdata

// helper functions for tests.

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// PrivateKey returns the rsa private key associated with RSAKey.
func PrivateKey() *rsa.PrivateKey {
	return key(RSAPrivateKey)
}

// PublicKey returns the rsa publickey
func PublicKey() *rsa.PublicKey {
	return pubkey(RSAPublicKey)
}

// AuthorityKey returns the rsa private key associated with authority.
func AuthorityKey() *rsa.PrivateKey {
	return key(Authority)
}

// ECCKey returns a test ecc key
func ECCKey() *ecdsa.PrivateKey {
	b, _ := base64.StdEncoding.DecodeString(ecc)
	key, _ := x509.ParseECPrivateKey(b)
	return key
}

// ECCPublicFromPrivateKey turns an rsa private key into a compressed
// ecdsa public key on the p256 curve. This is mostly used to make it easier
// to do things like create test entities.
func ECCPublicFromPrivateKey(k *rsa.PrivateKey) string {
	eccKey := wonka.ECCFromRSA(k)
	return wonka.KeyToCompressed(eccKey.PublicKey.X, eccKey.PublicKey.Y)
}

func key(k string) *rsa.PrivateKey {
	b, _ := base64.StdEncoding.DecodeString(k)
	key, _ := x509.ParsePKCS1PrivateKey(b)
	return key
}

func pubkey(k string) *rsa.PublicKey {
	b, e := base64.StdEncoding.DecodeString(k)
	if e != nil {
		panic(e)
	}

	pubKey, e := x509.ParsePKIXPublicKey(b)
	if e != nil {
		panic(e)
	}

	return pubKey.(*rsa.PublicKey)
}

// BodyContains retruns true if the http body contains the string
func BodyContains(body io.ReadCloser, contains string) (string, bool) {
	b, e := ioutil.ReadAll(body)
	if e != nil {
		panic(e)
	}

	bodyStr := string(b)
	return bodyStr, strings.Contains(bodyStr, contains)
}

// WithHTTPListener sets up and http listener and returns a mux where a handler
// can be added.
func WithHTTPListener(fn func(net.Listener, *xhttp.Router)) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}

	router := xhttp.NewRouter()

	go http.Serve(ln, router)

	fn(ln, router)
}

// WithWonkaMaster starts a wonka master listening on addr for incoming connections.
// TODO(jkline) The first argument to WithWonkaMaster used to be name, and was
// used to configure galileo client and jaeger tracer. Remove the argument
// entirely now that it is unused. Ref commits e74838b1, b448734c.
func WithWonkaMaster(_ string, fn func(r common.Router, handlerCfg common.HandlerConfig)) {
	// TODO(abg): Inject logger here
	log := zap.L()

	WithTempDir(func(dir string) {
		pubKeyPath := path.Join(dir, "pub")

		ln, e := net.Listen("tcp", "localhost:0")
		if e != nil {
			panic(fmt.Sprintf("listen: %v", e))
		}
		log.Info("wonkamaster listening", zap.Any("addr", ln.Addr().String()))

		h, p, err := net.SplitHostPort(ln.Addr().String())
		if err != nil {
			panic(err)
		}

		rsaKey := PrivateKey()
		if e := WritePublicKey(rsaKey.Public(), pubKeyPath); e != nil {
			panic(fmt.Sprintf("write pub key: %v", e))
		}

		eccKey := wonka.ECCFromRSA(rsaKey)

		os.Setenv("WONKA_MASTER_CERTPIN_N", fmt.Sprintf("%X", rsaKey.PublicKey.N))
		os.Setenv("WONKA_MASTER_CERTPIN_E", fmt.Sprintf("%06X", rsaKey.PublicKey.E))
		os.Setenv("WONKA_MASTER_PUBKEY_PATH", pubKeyPath)
		os.Setenv("WONKA_MASTER_HOST", h)
		os.Setenv("WONKA_MASTER_PORT", p)
		os.Setenv("WONKA_MASTER_ECC_PUB",
			wonka.KeyToCompressed(eccKey.PublicKey.X, eccKey.PublicKey.Y))
		defer os.Unsetenv("WONKA_MASTER_CERTPINT_N")
		defer os.Unsetenv("WONKA_MASTER_CERTPINT_E")
		defer os.Unsetenv("WONKA_MASTER_PUBKEY_PATH")
		defer os.Unsetenv("WONKA_MASTER_HOST")
		defer os.Unsetenv("WONKA_MASTER_PORT")
		defer os.Unsetenv("WONKA_MASTER_ECC_PUB")

		router := xhttp.NewRouter()

		go http.Serve(ln, router)

		log.Debug("setting evironment variables for testing",
			zap.String("WONKA_MASTER_CERTPIN_N", fmt.Sprintf("%X", rsaKey.PublicKey.N)),
			zap.String("WONKA_MASTER_CERTPIN_E", fmt.Sprintf("%06X", rsaKey.PublicKey.E)),
			zap.Any("WONKA_MASTER_PUBKEY_PATH", pubKeyPath),
			zap.Any("WONKA_MASTER_HOST", h),
			zap.Any("WONKA_MASTER_PORT", p),
		)

		err = wonka.InitWonkaMasterECC()
		if err != nil {
			panic(err)
		}

		var mem map[string][]string
		handlerCfg := common.HandlerConfig{
			Logger:     log,
			ECPrivKey:  eccKey,
			RSAPrivKey: rsaKey,
			DB:         wonkadb.NewMockEntityDB(),
			Metrics:    tally.NoopScope,
			Pullo:      rpc.NewMockPulloClient(mem),
		}

		fn(router, handlerCfg)
	})
}

// NewClaimReq returns a new claim request.
func NewClaimReq(name, claim string) wonka.ClaimRequest {
	return wonka.ClaimRequest{
		Version:     wonka.SignEverythingVersion,
		EntityName:  name,
		Claim:       claim,
		Destination: "*",
		Ctime:       time.Now().Unix(),
		Etime:       time.Now().Add(1 * time.Minute).Unix(),
		SigType:     "SHA256",
	}
}

func withUsshAgent(name string, certType uint32, fn func(string, ssh.PublicKey)) {
	log := zap.L() // TODO(abg): Inject logger here

	privKey := key(RSAPrivateKey)
	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		log.Fatal("error parsing rsa key", zap.Error(err))
	}

	email := name
	if strings.Contains(name, "@") {
		name = strings.Split(name, "@")[0]
	}

	c := &ssh.Certificate{
		CertType:        certType,
		Key:             pubKey,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
		ValidPrincipals: []string{name},
	}

	authority := key(Authority)
	signer, err := ssh.NewSignerFromKey(authority)
	if err != nil {
		log.Fatal("error parsing rsa key for signer", zap.Error(err))
	}

	switch certType {
	case ssh.UserCert:
		oldOwner := os.Getenv("UBER_OWNER")
		defer func() {
			if oldOwner != "" {
				os.Setenv("UBER_OWNER", oldOwner)
			}
		}()
		os.Setenv("UBER_OWNER", email)
		os.Setenv("WONKA_USSH_CA", string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	case ssh.HostCert:
		os.Setenv("WONKA_USSH_HOST_CA",
			fmt.Sprintf("@cert-authority * %s", ssh.MarshalAuthorizedKey(signer.PublicKey())))
	}

	if err := c.SignCert(rand.Reader, signer); err != nil {
		log.Fatal("error signing cert", zap.Error(err))
	}

	a := agent.NewKeyring()
	addedKey := agent.AddedKey{PrivateKey: key(RSAPrivateKey), Certificate: c}
	if err := a.Add(addedKey); err != nil {
		log.Fatal("error adding key to keyring", zap.Error(err))
	}

	oldSock := os.Getenv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")
	defer os.Setenv("SSH_AUTH_SOCK", oldSock)

	WithTempDir(func(dir string) {
		newSock := path.Join(dir, "ssh_auth_sock")
		os.Setenv("SSH_AUTH_SOCK", newSock)
		ln, err := net.Listen("unix", newSock)
		if err != nil {
			log.Fatal("error listening on new sock", zap.Error(err))
		}

		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					continue
				}
				go agent.ServeAgent(a, conn)
			}
		}()

		fn(newSock, signer.PublicKey())
	})
}

// WithUSSHAgent creates a new ssh-agent, adds a certificate and returns the
// CA key used to the sign the cert.
func WithUSSHAgent(name string, fn func(agentPath string, caKey ssh.PublicKey)) {
	withUsshAgent(name, ssh.UserCert, fn)
}

// WithUSSHHostAgent creates an ssh-agent and adds a host certificate to the agen
// and returns the CA key used to sign the cert.
func WithUSSHHostAgent(name string, fn func(agentPath string, caKey ssh.PublicKey)) {
	withUsshAgent(name, ssh.HostCert, fn)
}

// GenerateKeys is a test helper function that will write the public and private portions of
// k to pubPath and privPath.
func GenerateKeys(pubPath, privPath string, k *rsa.PrivateKey) error {
	b := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}

	if e := ioutil.WriteFile(privPath, pem.EncodeToMemory(&b),
		0644); e != nil {
		return e
	}
	pubBytes, e := x509.MarshalPKIXPublicKey(k.Public())
	if e != nil {
		return e
	}
	pub := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}

	return ioutil.WriteFile(pubPath, pem.EncodeToMemory(&pub), 0644)
}

// WithTempDir runs function in an ephemeral directory and cleans up after itself.
func WithTempDir(fn func(dir string)) {
	dir, err := ioutil.TempDir("", "wonka")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(dir)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	defer os.Chdir(cwd)
	os.Chdir(dir)

	fn(dir)
}

// WriteCertificate writes the given private key out to location as an x509 certificate.
func WriteCertificate(k *rsa.PrivateKey, loc string) error {
	t := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "foo",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(1 * time.Minute),

		SubjectKeyId:          []byte{1, 2, 3, 4},
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	cert, e := x509.CreateCertificate(rand.Reader, &t, &t, k.Public(), k)
	if e != nil {
		panic(fmt.Sprintf("fak: %v", e))
	}

	certPem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	}

	return ioutil.WriteFile(loc, pem.EncodeToMemory(certPem), 0440)
}

// WritePublicKey writes the publickey to loc in pem format.
func WritePublicKey(k crypto.PublicKey, loc string) error {
	b, e := x509.MarshalPKIXPublicKey(k)
	if e != nil {
		return e
	}

	pemBlock := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: b,
	}

	e = ioutil.WriteFile(loc, pem.EncodeToMemory(&pemBlock), 0660)
	if e != nil {
		return e
	}
	return nil
}

// WritePrivateKey writes the given privatekey to the given location as a pem key.
func WritePrivateKey(k *rsa.PrivateKey, loc string) error {
	pemBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}
	e := ioutil.WriteFile(loc, pem.EncodeToMemory(&pemBlock), 0660)
	if e != nil {
		return e
	}
	return nil
}
