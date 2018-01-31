package main

import (
	"crypto"
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

// Streamlines creating a temporary file at a given path
func createTempFile(t *testing.T, path string) (tempFile *os.File, remover func()) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	require.NoError(t, err, "making a temp file should not fail")
	return f, func() { os.Remove(path) }
}

func TestLoadHostCert(t *testing.T) {
	var hostCertVars = []struct {
		name string
	}{{name: "foo01"}}

	for _, m := range hostCertVars {
		withSSHConfig(t, m.name, func(configPath string) {
			cert, privKey, err := usshHostCert(zap.NewNop(), configPath)
			require.NoError(t, err, "error reading host cert and key: %v", err)
			require.Equal(t, cert.ValidPrincipals[0], m.name)

			signer, err := ssh.NewSignerFromKey(privKey)
			require.NoError(t, err, "error getting signing key: %v", err)

			// now verify the cert and privkey corespond to each other.
			data, sig := signData(signer)
			err = cert.Verify(data, sig)
			require.NoError(t, err, "verify error: %v", err)
		})
	}
}

func signData(privKey ssh.Signer) ([]byte, *ssh.Signature) {
	b := make([]byte, 128)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	sig, err := privKey.Sign(rand.Reader, b)
	if err != nil {
		panic(err)
	}

	return b, sig
}

func TestPrivKeyFromPathRSA(t *testing.T) {
	path := filepath.Join(os.TempDir(), "ssh_host_rsa_key")
	f, rmTempFile := createTempFile(t, path)
	defer rmTempFile()

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating rsa keys should not fail")

	// write key to file
	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	privPem := pem.EncodeToMemory(privBlock)
	f.Write(privPem)

	pubKey, err := ssh.NewPublicKey(&key.PublicKey)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	// create a faux ssh cert
	cert := ssh.Certificate{Key: pubKey}

	sshKey, err := privKeyFromPath(&cert, []string{path})
	require.NoError(t, err, "finds private key")
	require.NotNil(t, sshKey, "key should not be nil")
}

func TestPrivKeyFromPathDSA(t *testing.T) {
	path := filepath.Join(os.TempDir(), "ssh_host_dsa_key")
	f, rmTempFile := createTempFile(t, path)
	defer rmTempFile()

	params := new(dsa.Parameters)
	err := dsa.GenerateParameters(params, rand.Reader, dsa.L1024N160)
	require.NoError(t, err, "generating dsa params should not fail")
	dsaKey := new(dsa.PrivateKey)
	dsaKey.PublicKey.Parameters = *params
	err = dsa.GenerateKey(dsaKey, rand.Reader)
	require.NoError(t, err, "generating dsa keys should not fail")

	dsaParams := struct {
		Version       int
		P, Q, G, Y, X *big.Int
	}{
		0, dsaKey.P, dsaKey.Q, dsaKey.G, dsaKey.Y, dsaKey.X,
	}
	asn1Bytes, err := asn1.Marshal(dsaParams)
	require.NoError(t, err, "marshalling dsa keys to asn1 should not fail")

	// write key to file
	privBlock := &pem.Block{
		Type:  "DSA PRIVATE KEY",
		Bytes: asn1Bytes,
	}
	privPem := pem.EncodeToMemory(privBlock)
	f.Write(privPem)

	pubKey, err := ssh.NewPublicKey(&dsaKey.PublicKey)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	// create a faux ssh cert
	cert := ssh.Certificate{Key: pubKey}

	sshKey, err := privKeyFromPath(&cert, []string{path})
	require.NoError(t, err, "finds private key")
	require.NotNil(t, sshKey, "key should not be nil")
}

func TestPrivKeyFromPathECDSA(t *testing.T) {
	path := filepath.Join(os.TempDir(), "ssh_host_ecdsa_key")
	f, rmTempFile := createTempFile(t, path)
	defer rmTempFile()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generating ecdsa keys should not fail")

	bytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err, "marshalling ecdsa private key should not fail")

	// write key to file
	privBlock := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: bytes,
	}
	privPem := pem.EncodeToMemory(privBlock)
	f.Write(privPem)

	pubKey, err := ssh.NewPublicKey(&key.PublicKey)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	// create a faux ssh cert
	cert := ssh.Certificate{Key: pubKey}

	sshKey, err := privKeyFromPath(&cert, []string{path})
	require.NoError(t, err, "finds private key")
	require.NotNil(t, sshKey, "key should not be nil")
}

func TestPrivKeyFromPathEd25519(t *testing.T) {
	path := filepath.Join(os.TempDir(), "ssh_host_ed25519_key")
	f, rmTempFile := createTempFile(t, path)
	defer rmTempFile()

	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "generating ed25519 keys should not fail")

	// write key to file
	privBlock := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: MarshalED25519PrivateKey(edPriv),
	}
	privPem := pem.EncodeToMemory(privBlock)
	f.Write(privPem)

	pubKey, err := ssh.NewPublicKey(edPub)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	// create a faux ssh cert
	cert := ssh.Certificate{Key: pubKey}

	sshKey, err := privKeyFromPath(&cert, []string{path})
	require.NoError(t, err, "finds private key")
	require.NotNil(t, sshKey, "key should not be nil")
}

func withSSHConfig(t *testing.T, name string, fn func(string)) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)
	cert, privKey := createCert(name, signer)

	defer testhelper.SetEnvVar("WONKA_USSH_HOST_CA",
		fmt.Sprintf("@cert-authority * %s", ssh.MarshalAuthorizedKey(signer.PublicKey())))()

	rsaPriv, ok := privKey.(*rsa.PrivateKey)
	require.True(t, ok, "privkey not an rsa key")

	wonkatestdata.WithTempDir(func(dir string) {
		certPath := path.Join(dir, "ssh_host_rsa_key-cert.pub")

		err := ioutil.WriteFile(certPath, ssh.MarshalAuthorizedKey(cert), 0666)
		require.NoError(t, err, "error writing cert: %v", err)

		privKeyPath := path.Join(dir, "ssh_host_rsa_key")
		pemBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaPriv),
		}
		err = ioutil.WriteFile(privKeyPath, pem.EncodeToMemory(pemBlock), 0666)
		require.NoError(t, err, "error writing private key: %v", err)

		configPath := path.Join(dir, "sshd_config")
		configLine := fmt.Sprintf("HostCertificate %s\nHostKey %s", certPath, privKeyPath)

		err = ioutil.WriteFile(configPath, []byte(configLine), 0666)
		require.NoError(t, err, "error writing config file: %v", err)

		oldSSHDconfig := sshdConfig
		sshdConfig = &configPath
		defer func() { sshdConfig = oldSSHDconfig }()

		fn(configPath)
	})

}

func TestPrivKeyFromPathInvalidPathShouldError(t *testing.T) {
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "generating ed25519 keys should not fail")

	pubKey, err := ssh.NewPublicKey(edPub)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	// create a faux ssh cert
	cert := ssh.Certificate{Key: pubKey}

	_, err = privKeyFromPath(&cert, []string{""})
	require.Error(t, err, "empty path should fail")
}

func TestPrivKeyFromPathInvalidFileContentsShouldError(t *testing.T) {
	path := filepath.Join(os.TempDir(), "ssh_host_ed25519_key")
	f, rmTempFile := createTempFile(t, path)
	defer rmTempFile()

	f.WriteString("I'm not a valid ed25519 private key.")

	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "generating ed25519 keys should not fail")

	pubKey, err := ssh.NewPublicKey(edPub)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	// create a faux ssh cert
	cert := ssh.Certificate{Key: pubKey}

	_, err = privKeyFromPath(&cert, []string{path})
	require.Error(t, err, "private key is invalid")
}

func createCert(name string, signer ssh.Signer) (*ssh.Certificate, crypto.PrivateKey) {
	privKey := wonkatestdata.PrivateKey()

	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(err)
	}

	c := &ssh.Certificate{
		Key:             pubKey,
		CertType:        ssh.HostCert,
		ValidPrincipals: []string{name},
		Serial:          0,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
		ValidAfter:      uint64(time.Now().Add(-time.Minute).Unix()),
	}

	if err := c.SignCert(rand.Reader, signer); err != nil {
		panic(err)
	}

	return c, privKey
}

// Borrowed from https://github.com/mikesmitty/edkey/blob/master/edkey.go
// and modified slightly for testing only
func MarshalED25519PrivateKey(key ed25519.PrivateKey) []byte {
	magic := append([]byte("openssh-key-v1"), 0)

	var w struct {
		CipherName   string
		KdfName      string
		KdfOpts      string
		NumKeys      uint32
		PubKey       []byte
		PrivKeyBlock []byte
	}

	pk1 := struct {
		Check1  uint32
		Check2  uint32
		Keytype string
		Pub     []byte
		Priv    []byte
		Comment string
		Pad     []byte `ssh:"rest"`
	}{}

	ci := uint32(0)
	pk1.Check1 = ci
	pk1.Check2 = ci

	pk1.Keytype = ssh.KeyAlgoED25519

	pk, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil
	}
	pubKey := []byte(pk)
	pk1.Pub = pubKey

	pk1.Priv = []byte(key)

	pk1.Comment = "Sad panda."

	bs := 8
	blockLen := len(ssh.Marshal(pk1))
	padLen := (bs - (blockLen % bs)) % bs
	pk1.Pad = make([]byte, padLen)

	for i := 0; i < padLen; i++ {
		pk1.Pad[i] = byte(i + 1)
	}

	prefix := []byte{0x0, 0x0, 0x0, 0x0b}
	prefix = append(prefix, []byte(ssh.KeyAlgoED25519)...)
	prefix = append(prefix, []byte{0x0, 0x0, 0x0, 0x20}...)

	w.CipherName = "none"
	w.KdfName = "none"
	w.KdfOpts = ""
	w.NumKeys = 1
	w.PubKey = append(prefix, pubKey...)
	w.PrivKeyBlock = ssh.Marshal(pk1)

	magic = append(magic, ssh.Marshal(w)...)

	return magic
}
