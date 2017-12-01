package claims

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"go.uber.org/zap"
)

// TODO(pmoody): pretty much everything here should be moved to wonka-go

// NewSignedClaim generates a new signed claim
// TODO(pmoody): move this to wonka-go
func NewSignedClaim(c wonka.ClaimRequest, ecPrivKey *ecdsa.PrivateKey, pubKey crypto.PublicKey) (string, error) {
	// TODO(abg): Inject logger here
	log := zap.L()

	claim := wonka.Claim{
		ClaimType:   "WONKAC",
		ValidAfter:  c.Ctime,
		ValidBefore: c.Etime,
		EntityName:  c.EntityName,
		Claims:      strings.Split(c.Claim, ","),
		Destination: c.Destination,
	}

	toHash, err := json.Marshal(claim)
	if err != nil {
		log.Error("marshalling claim to sign", zap.Error(err))
		return "", err
	}

	// Hash the claim manifest for signing
	echasher := sha256.New()
	echasher.Write([]byte(toHash))
	entityHash := echasher.Sum(nil)

	// Sign the claim token using the ECC private key
	r, s, err := ecdsa.Sign(rand.Reader, ecPrivKey, entityHash)
	if err != nil {
		log.Error("signing claim",
			zap.Any("entity", claim.EntityName),
			zap.Error(err),
		)

		return "", err
	}

	// Append the ECC signature to the chosen R parameter to the claim token
	rpad := 32 - len(r.Bytes())
	spad := 32 - len(s.Bytes())

	// Pad R Field to 32-bytes if needed
	for i := 0; i < rpad; i++ {
		claim.Signature = append(claim.Signature, 0x00)
	}
	claim.Signature = append(claim.Signature, r.Bytes()...)

	// Pad S Field to 32-bytes if needed
	for i := 0; i < spad; i++ {
		claim.Signature = append(claim.Signature, 0x00)
	}
	claim.Signature = append(claim.Signature, s.Bytes()...)

	claimBytes, err := json.Marshal(claim)
	if err != nil {
		log.Error("marshalling claim for encryption",
			zap.Any("entity", c.EntityName),
			zap.Error(err),
		)

		return "", err
	}

	var encryptedClaim []byte
	switch t := pubKey.(type) {
	case *ecdsa.PublicKey:
		encryptedClaim, err = wonkacrypter.New().Encrypt(claimBytes, ecPrivKey, t)
		if err != nil {
			log.Error("ECIES encrypting claim for entity",
				zap.Error(err),
				zap.Any("entity", c.EntityName),
			)

			return "", nil
		}
	case *rsa.PublicKey:
		encryptedClaim, err = rsa.EncryptPKCS1v15(rand.Reader, t, claimBytes)
		if err != nil {
			log.Error("encrypting claim for entity",
				zap.Any("entity", c.EntityName),
				zap.Error(err),
			)

			return "", err
		}
	}

	// Base64 encode the encrypted reply
	return base64.StdEncoding.EncodeToString(encryptedClaim), nil
}

// SignClaimRequest signs a claim request.
// TODO(pmoody): replace this with wonka-go
func SignClaimRequest(cr *wonka.ClaimRequest, k *rsa.PrivateKey) error {
	log := zap.L() // TODO(abg): Inject logger here

	toSign, err := json.Marshal(cr)
	if err != nil {
		log.Error("marshalling claim", zap.Error(err))
		return err
	}

	var hasher crypto.Hash
	switch cr.SigType {
	case "SHA1":
		hasher = crypto.SHA1
	case "SHA256":
		hasher = crypto.SHA256
	default:
		return fmt.Errorf("unknown hash algorithm: %s", cr.SigType)
	}

	mhash := hasher.New()
	mhash.Write([]byte(toSign))
	pkhash := mhash.Sum(nil)

	sigBytes, err := k.Sign(rand.Reader, pkhash, hasher)
	if err != nil {
		return fmt.Errorf("sign error: %v", err)
	}

	cr.Signature = string(base64.StdEncoding.EncodeToString(sigBytes))
	keyBytes, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		return fmt.Errorf("error marshaling x509 pubkey: %v", err)
	}
	cr.SessionPubKey = base64.StdEncoding.EncodeToString(keyBytes)

	return nil
}

// VerifyClaimRequest verifies that the given claim request came from the given wonka entity.
func VerifyClaimRequest(version int, cr wonka.ClaimRequest, e wonka.Entity) (crypto.PublicKey, string, error) {
	log := zap.L() // TODO(abg): Inject logger here

	var pubKey crypto.PublicKey
	var err error
	switch version {
	case 2:
		ecPubKey, err := wonka.KeyFromCompressed(e.ECCPublicKey)
		if err != nil {
			log.Error("couldn't pull out ecc key", zap.Error(err))
			return nil, "", err
		}
		pubKey = ecPubKey
	default:
		pubKey, err = keys.ParsePublicKey(e.PublicKey)
		if err != nil {
			log.Error("couldn't parse rsa key", zap.Error(err))
			return nil, "", err
		}
	}

	toVerify := []byte(fmt.Sprintf("%s<%d|%d>%s|%s",
		cr.EntityName, cr.Ctime, cr.Etime, cr.Claim, cr.Destination))
	if cr.Version == wonka.SignEverythingVersion {
		// for signature verification, we copy the claim request and clear the unsigned fields.
		toVerify, err = json.Marshal(ClaimRequestForVerify(cr))
		if err != nil {
			log.Error("json marshal", zap.Error(err))
			return nil, "", err
		}
	}

	log.Debug("verifying signature",
		zap.Any("version", version),
		zap.Any("key", keys.KeyHash(pubKey)),
		zap.Any("toVerify", toVerify),
		zap.Any("signature", cr.Signature),
	)

	if err := keys.VerifySignature(pubKey, cr.Signature, cr.SigType, string(toVerify)); err != nil {
		log.Warn("error verifying request",
			zap.Any("entity", e.EntityName),
			zap.Error(err),
		)
		return nil, "", err
	}

	return pubKey, string(toVerify), nil
}

// ClaimRequestForVerify returns a copy of the given claim request with the signature bits
// stripped out so it's ready for signature verification.
// TODO(pmoody): this should probably be moved to wonka-go.
func ClaimRequestForVerify(cr wonka.ClaimRequest) wonka.ClaimRequest {
	verifyCR := ClaimRequestForUSSHVerify(cr)
	verifyCR.Signature = ""
	verifyCR.SessionPubKey = ""
	verifyCR.USSHCertificate = ""
	return verifyCR
}

// ClaimRequestForUSSHVerify returns a copy of the given claim request with the signature bits
// stripped out so it's ready for ussh signature verification.
// TODO(pmoody): this should probably be moved to wonka-go.
func ClaimRequestForUSSHVerify(cr wonka.ClaimRequest) wonka.ClaimRequest {
	verifyCR := cr
	verifyCR.USSHSignature = ""
	verifyCR.USSHSignatureType = ""
	return verifyCR
}
