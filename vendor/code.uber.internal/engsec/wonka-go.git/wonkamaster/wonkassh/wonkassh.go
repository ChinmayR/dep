package wonkassh

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// CertFromRequest returns the base64 encodied ssh key as an ssh certificate if it's a ussh
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

	// Check the USSH certificate against the CA for validity
	certChecker := ssh.CertChecker{
		IsUserAuthority: func(k ssh.PublicKey) bool {
			for _, ca := range usshCAKeys {
				if bytes.Equal(k.Marshal(), ca.Marshal()) {
					return true
				}
			}
			return false
		},
	}

	// Run the cert check - allow any principal but still run all other checks
	if err := certChecker.CheckCert(cert.ValidPrincipals[0], cert); err != nil {
		return nil, fmt.Errorf("cert check failed: %v", err)
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
