package claimhelper

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
)

// Encapsulating the token unmarshal with check or validate is very useful for
// testing. And, we want to keep these out of the public API.
// Eventually we'll have a ClaimCheckerService, or similar, to replace these.

// ClaimCheck unmarshals and validates a claim token.
// See Claim.Check for details.
func ClaimCheck(allowedClaims []string, dest, claimToken string) error {
	claim, err := wonka.UnmarshalClaim(claimToken)
	if err != nil {
		return err
	}
	return claim.Check(dest, allowedClaims)
}

// ClaimValidate unmarshals and validates a claim token.
// See Claim.Validate for details.
func ClaimValidate(claimToken string) error {
	claim, err := wonka.UnmarshalClaim(claimToken)
	if err != nil {
		return err
	}
	return claim.Validate()
}

// NewSignedClaim generates a new signed claim
func NewSignedClaim(c wonka.ClaimRequest, ecPrivKey *ecdsa.PrivateKey) (*wonka.Claim, error) {
	claim := &wonka.Claim{
		ClaimType:   "WONKAC",
		ValidAfter:  c.Ctime,
		ValidBefore: c.Etime,
		EntityName:  c.EntityName,
		Claims:      strings.Split(c.Claim, ","),
		Destination: c.Destination,
	}

	toSign, err := json.Marshal(claim)
	if err != nil {
		return nil, fmt.Errorf("error marshalling claim to sign: %v", err)
	}

	claim.Signature, err = wonkacrypter.New().Sign(toSign, ecPrivKey)
	if err != nil {
		return nil, err
	}

	return claim, nil
}

// EncryptClaim encrypts a claim for the given remote entity
func EncryptClaim(claim *wonka.Claim, ecPrivKey *ecdsa.PrivateKey, pubKey crypto.PublicKey) (string, error) {
	claimBytes, err := json.Marshal(claim)
	if err != nil {
		return "", fmt.Errorf("error marshalling claim for encryption: %v", err)
	}

	if ecPrivKey == nil {
		return "", errors.New("nil private key")
	}

	if pubKey == nil {
		return "", errors.New("nil public key")
	}

	var encryptedClaim []byte
	switch t := pubKey.(type) {
	case *ecdsa.PublicKey:
		encryptedClaim, err = wonkacrypter.New().Encrypt(claimBytes, ecPrivKey, t)
		if err != nil {
			return "", fmt.Errorf("error encrypting claim: %v", err)
		}
	case *rsa.PublicKey:
		// this should probably go away
		encryptedClaim, err = rsa.EncryptPKCS1v15(rand.Reader, t, claimBytes)
		if err != nil {
			return "", fmt.Errorf("error encrypting claim: %v", err)
		}
	}

	// Base64 encode the encrypted reply
	return base64.StdEncoding.EncodeToString(encryptedClaim), nil
}
