package wonkassh

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"net"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestCertFromRequestInvalidCertShouldError(t *testing.T) {
	_, err := CertFromRequest("invalid", nil)
	require.Error(t, err, "invalid cert should error")
}

func TestCertFromRequestNotCertShouldError(t *testing.T) {
	privKey := wonkatestdata.PrivateKey()
	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	require.NoError(t, err, "should not error making ssh public key")

	marshaledKey := ssh.MarshalAuthorizedKey(pubKey)

	_, err = CertFromRequest(string(marshaledKey), nil)
	require.Error(t, err, "non-certificate should error")
}

func TestCertFromRequestCertCheckFailsShouldError(t *testing.T) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)
	expectedCert, _ := createCert("cert_from_request", signer)
	expectedCert.Signature = &ssh.Signature{Format: "ssh-rsa"}

	marshaledCert := ssh.MarshalAuthorizedKey(expectedCert)

	_, err = CertFromRequest(string(marshaledCert), nil)
	require.Error(t, err, "invalid cert should fail cert check")
}

func TestCertFromRequest(t *testing.T) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)

	pubKey, err := ssh.NewPublicKey(&authority.PublicKey)
	require.NoError(t, err, "creating ssh public key should not error")

	expectedCert, _ := createCert("cert_from_request", signer)
	marshaledCert := ssh.MarshalAuthorizedKey(expectedCert)

	resultCert, err := CertFromRequest(string(marshaledCert), []ssh.PublicKey{pubKey})
	require.NoError(t, err, "should result in same cert")
	require.Equal(t, expectedCert.Key, resultCert.Key)
}

func TestVerifyUSSHSignatureInvalidSigShouldError(t *testing.T) {
	err := VerifyUSSHSignature(nil, "", "invalid", "")
	require.Error(t, err, "invalid signature should error")
}

func TestVerifyUSSHSignature(t *testing.T) {
	authority := wonkatestdata.AuthorityKey()
	rootSigner, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating root signer: %v", err)

	cert, _ := createCert("verify_ussh_signature", rootSigner)
	signer, err := ssh.NewSignerFromKey(wonkatestdata.PrivateKey())
	require.NoError(t, err, "error creating intermediate signer: %v", err)

	sig, err := signer.Sign(rand.Reader, []byte("hello"))
	require.NoError(t, err, "signing data does not error")

	encodedSig := base64.StdEncoding.EncodeToString(sig.Blob)
	err = VerifyUSSHSignature(cert, "hello", encodedSig, sig.Format)
	require.NoError(t, err, "signature should verify")
}

func TestCheckUserCertInvalidNameShouldError(t *testing.T) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)

	cert, _ := createCert("check_user_cert", signer)
	err = CheckUserCert("foo", cert, nil)
	require.Error(t, err, "should error with invalid name")
}

func TestCheckUserCert(t *testing.T) {
	authority := wonkatestdata.AuthorityKey()
	signer, err := ssh.NewSignerFromKey(authority)
	require.NoError(t, err, "error creating signer: %v", err)

	pubKey, err := ssh.NewPublicKey(&authority.PublicKey)
	require.NoError(t, err, "should not error making ssh public key")

	cert, _ := createCert("thom@uber.com", signer)
	err = CheckUserCert("thom@uber.com", cert, []ssh.PublicKey{pubKey})
	require.NoError(t, err, "should not error")
}

func TestConnMD(t *testing.T) {
	c := connMD{user: "foober@uber.com"}
	addr := &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}

	require.Equal(t, c.user, c.User(), "users must be equal")
	require.Nil(t, c.SessionID())
	require.Nil(t, c.ClientVersion())
	require.Nil(t, c.ServerVersion())
	require.Equal(t, addr, c.RemoteAddr())
	require.Equal(t, addr, c.LocalAddr())
}

func createCert(name string, signer ssh.Signer) (*ssh.Certificate, crypto.PrivateKey) {
	privKey := wonkatestdata.PrivateKey()

	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(err)
	}

	c := &ssh.Certificate{
		Key:             pubKey,
		CertType:        ssh.UserCert,
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
