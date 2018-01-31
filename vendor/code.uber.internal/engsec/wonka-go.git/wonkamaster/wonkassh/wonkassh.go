package wonkassh

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// Returns a function which takes a key and compares it against the supplied
// list of caKeys. The result is intended to be provided as IsUserAuthority.
func isUserAuthority(caKeys []ssh.PublicKey) func(ssh.PublicKey) bool {
	return func(k ssh.PublicKey) bool {
		for _, ca := range caKeys {
			if bytes.Equal(k.Marshal(), ca.Marshal()) {
				return true
			}
		}
		return false
	}
}

// CertFromRequest returns the base64 encoded ssh key as an ssh certificate if it's a ussh
// certificate.
// TODO(pmoody): pull this from ussh.git instead
func CertFromRequest(certificate string, usshCAKeys []ssh.PublicKey) (*ssh.Certificate, error) {
	log := zap.L() // TODO(abg): Inject logger here

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(certificate))
	if err != nil {
		log.Error("error parsing certificate", zap.Error(err))
		return nil, fmt.Errorf("parsing ceritifcate: %v", err)
	}
	cert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("rejecting non-cert request")
	}

	if err := CheckUserCert(cert.ValidPrincipals[0], cert, usshCAKeys); err != nil {
		return nil, err
	}

	return cert, nil
}

// VerifyUSSHSignature verifies that sig was generated with cert's private key.
func VerifyUSSHSignature(cert *ssh.Certificate, toVerify, sig, sigType string) error {
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("signature decoding error: %v", err)
	}

	sshSig := &ssh.Signature{
		Format: sigType,
		Blob:   sigBytes,
	}

	return cert.Key.Verify([]byte(toVerify), sshSig)
}

// CheckUserCert verifies that the ssh user certificate was signed by an accepted ssh certificate authority.
func CheckUserCert(name string, cert *ssh.Certificate, caKeys []ssh.PublicKey) error {
	// Check the USSH certificate against the CA for validity
	certChecker := ssh.CertChecker{
		IsUserAuthority: isUserAuthority(caKeys),
	}

	c := connMD{user: name}
	if _, err := certChecker.Authenticate(c, cert); err != nil {
		return fmt.Errorf("error authenticating: %v", err)
	}

	return nil
}

// connMD implements the ssh.ConnMetadata interface
type connMD struct {
	user string
}

func (c connMD) User() string {
	return c.user
}

func (connMD) SessionID() []byte {
	return nil
}

func (connMD) ClientVersion() []byte {
	return nil
}

func (connMD) ServerVersion() []byte {
	return nil
}

func (connMD) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}
}

func (connMD) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 22}
}
