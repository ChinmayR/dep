package cmd

import (
	"bytes"
	"context"
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"flag"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"github.com/urfave/cli"
)

// Streamlines creating a temporary file at a given path
func createTempFile(t *testing.T, path string) (tempFile *os.File, remover func()) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	require.NoError(t, err, "making a temp file should not fail")
	return f, func() { os.Remove(path) }
}

// Configure a cliWrapper with a context.Context in app metadata so we don't
// segfault in tests when NewWonkaClientFromConfig calls wonka.InitWithContext
func newCliWrapper() cliWrapper {
	a := cli.NewApp()
	a.Setup()
	a.Metadata["ctx"] = context.Background()
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(a, fs, nil)
	return cliWrapper{inner: c}
}

// Use this function to create a temporary wonka cert.
func createTestCert(t *testing.T) (*wonka.Certificate, *ecdsa.PrivateKey, *ecdsa.PrivateKey, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate key: %v", err)

	keyBytes, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	require.NoError(t, err, "marshalling pubkey: %v", err)

	c := &wonka.Certificate{
		EntityName:  "test_cert",
		Host:        "test_host",
		Key:         keyBytes,
		ValidAfter:  uint64(time.Now().Unix()),
		ValidBefore: uint64(time.Now().Add(time.Minute).Unix()),
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating key: %v", err)

	oldWonkaMasterKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&priv.PublicKey}

	err = c.SignCertificate(priv)
	require.NoError(t, err, "error signing cert: %v", err)

	pubKey, err := c.PublicKey()
	require.NoError(t, err, "getting key shouldn't error: %v", err)

	origKey, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	require.NoError(t, err, "marshalling pubkey: %v", err)
	newKey, err := x509.MarshalPKIXPublicKey(pubKey)
	require.NoError(t, err, "marshalling new pubkey: %v", err)
	require.True(t, bytes.Equal(origKey, newKey), "keys should equal")
	wonka.WonkaMasterPublicKeys = oldWonkaMasterKeys
	return c, k, priv, nil
}

func TestNewWonkaClientFromConfigWhenConfigIsEmptyThenShouldError(t *testing.T) {
	c := newCliWrapper()
	wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
		w, err := c.NewWonkaClientFromConfig(wonka.Config{})
		require.Nil(t, w)
		require.Error(t, err)
	})
}

func TestGenerateKeys(t *testing.T) {
	defer func() {
		os.Remove("wonka_public")
		os.Remove("wonka_private")
	}()

	privPem, pubPem, err := generateKeys()
	require.NotNil(t, privPem)
	require.NotNil(t, pubPem)
	require.NoError(t, err)
}

func TestNewWonkaClientFromConfigWorksCorrectly(t *testing.T) {
	c := newCliWrapper()

	testdata.WithTempDir(func(dir string) {
		k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := testdata.WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			pubKey := testdata.PublicPemFromKey(testdata.PrivateKeyFromPem(testdata.RSAPrivKey))
			e := wonka.Entity{
				EntityName:   "helper-test",
				ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
				PublicKey:    string(pubKey),
			}
			err := handlerCfg.DB.Create(context.TODO(), &e)
			require.NoError(t, err, "create entity")

			cfg := wonka.Config{
				EntityName:     "helper-test",
				PrivateKeyPath: privKeyPath,
			}

			w, err := c.NewWonkaClientFromConfig(cfg)
			require.NotNil(t, w)
			require.NoError(t, err)
		})
	})
}

func TestCertFromPathInvalidKeyShouldError(t *testing.T) {
	f, err := ioutil.TempFile("", "certFromPath")
	require.NoError(t, err, "creating tempfile should work")

	defer os.Remove(f.Name())
	_, err = certFromPath(f.Name())
	require.Error(t, err, "empty key should fail")
}

func TestCertFromPathInvalidPathShouldError(t *testing.T) {
	f, err := ioutil.TempFile("", "certFromPath")
	require.NoError(t, err, "making a temp file should not fail")

	// delete the file
	name := f.Name()
	os.Remove(name)

	_, err = certFromPath(name)
	require.Error(t, err, "non-existent path should fail")
}

func TestCertFromPathInvalidCertShouldError(t *testing.T) {
	f, err := ioutil.TempFile("", "certFromPath")
	require.NoError(t, err, "making a temp file should not fail")
	name := f.Name()
	defer os.Remove(name)

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err, "generating keys should not fail")

	pubKey, err := ssh.NewPublicKey(&key.PublicKey)
	require.NoError(t, err, "making generated keys usable by ssh should not fail")

	f.Write(ssh.MarshalAuthorizedKey(pubKey))

	sshKey, err := certFromPath(name)
	require.Error(t, err, "key is not cert")
	require.Nil(t, sshKey, "key should be nil")
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

func TestUsshHostCert(t *testing.T) {
	f, err := ioutil.TempFile("", "usshHostCert")
	require.NoError(t, err, "making a temp file should not fail")
	name := f.Name()
	defer os.Remove(name)

	f.WriteString("HostCertificate notValid\nHostKey notValid\n")

	_, _, err = usshHostCert(name)
	require.Error(t, err, "invalid host cert and key should error")
}

func TestNewWonkaClientWorksCorrectly(t *testing.T) {
	tests := []WonkaClientType{
		EnrollmentClient,
		EnrollmentClientGenerateKeys,
		ImpersonationClient,
		TaskLauncherClient,
	}
	for _, wonkaClientType := range tests {
		testdata.WithTempDir(func(dir string) {
			k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
			privKeyPath := path.Join(dir, "wonka_private.pem")
			e := testdata.WritePrivateKey(k, privKeyPath)
			require.NoError(t, e, "writing privkey")

			wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
				handlers.SetupHandlers(r, handlerCfg)
				require.NotNil(t, r, "setuphandlers returned nil")

				pubKey := testdata.PublicPemFromKey(testdata.PrivateKeyFromPem(testdata.RSAPrivKey))
				e := wonka.Entity{
					EntityName:   "helper-test",
					ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
					PublicKey:    string(pubKey),
				}
				err := handlerCfg.DB.Create(context.TODO(), &e)
				require.NoError(t, err, "create entity")

				cfg := wonka.Config{
					EntityName:     "helper-test",
					PrivateKeyPath: privKeyPath,
					Metrics:        tally.NoopScope,
				}

				fs := flag.NewFlagSet(
					"testing",
					flag.ContinueOnError,
				)

				fs.String("private-key", privKeyPath, "")
				fs.Parse([]string{""})
				app := cli.App{
					Metadata: map[string]interface{}{
						"config": cfg,
						"ctx":    context.Background(),
					},
				}
				cliCtx := cli.NewContext(&app, fs, nil)
				c := cliWrapper{inner: cliCtx}

				ctx, err := c.NewWonkaClient(wonkaClientType)
				require.Nil(t, ctx)
				require.Error(t, err)
			})
		})
	}
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
