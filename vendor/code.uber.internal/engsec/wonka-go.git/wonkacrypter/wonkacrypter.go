package wonkacrypter

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"math/big"

	"go.uber.org/zap"
)

// EntityCrypter is an interface that essentially wraps Crypter and uses the
// entity's private key when required for cryptographic operations.
type EntityCrypter interface {
	// Encrypt AES encrypts plainText using a block cipher with a shared key dervied
	// via ECDH using the other entity's public key.
	Encrypt(plainText []byte, ecPub *ecdsa.PublicKey) ([]byte, error)

	// Decrypt decrypts an aes encrypted with Encrypt()
	Decrypt(cipherText []byte, ecPub *ecdsa.PublicKey) ([]byte, error)

	// Sign ecdsa signs the SHA256 hash of data.
	Sign(data []byte) ([]byte, error)

	// Verify ecdsa verifies that the SHA256 hash of the given data was signed by
	// the private portion of the given ecdsa public key.
	Verify(data, sig []byte, ecPub *ecdsa.PublicKey) bool
}

// Crypter is an interface that defines the wonka crypto operations if you already have
// an entity's public key. Crypter is doing ECIES, the KDF is concatKDF, the symmetric
// cipher is AES256, and the MAC is GCM.
// TODO(pmoody): move this to ucrypto when they're ready to onboard.
type Crypter interface {
	// Encrypt AES encrypts plainText using a block cipher with a shared key dervied
	// via ECDH from the two given keys.
	Encrypt(plainText []byte, ecPriv *ecdsa.PrivateKey, ecPub *ecdsa.PublicKey) ([]byte, error)

	// Decrypt decrypts an aes encrypted with Encrypt()
	Decrypt(cipherText []byte, ecPriv *ecdsa.PrivateKey, ecPub *ecdsa.PublicKey) ([]byte, error)

	// Sign ecdsa signs the SHA256 hash of data with given ecdsa private key.
	Sign(data []byte, ecPriv *ecdsa.PrivateKey) ([]byte, error)

	// Verify ecdsa verifies that the SHA256 hash of the given data was signed by
	// the private portion of the given ecdsa public key.
	Verify(data, sig []byte, ecPub *ecdsa.PublicKey) bool
}

type wonkaCrypter struct {
	log *zap.Logger
}

type wonkaEntityCrypter struct {
	p *ecdsa.PrivateKey
	c Crypter
}

const (
	// ecc signatures are made up of an r and and s part and they should each
	// be this long.
	sigLen = 32
)

var (
	big2To32   = new(big.Int).Exp(big.NewInt(2), big.NewInt(32), nil)
	big2To32M1 = new(big.Int).Sub(big2To32, big.NewInt(1))
)

// New returns a new Crypter
func New() Crypter {
	// TODO(abg): Logger should be injected into New.
	return wonkaCrypter{log: zap.L()}
}

// VerifyAny calls verify on the slice of ec pubkeys
func VerifyAny(data, sig []byte, ecPubs []*ecdsa.PublicKey) bool {
	c := New()
	for _, k := range ecPubs {
		if ok := c.Verify(data, sig, k); ok {
			return true
		}
	}
	return false
}

// DecryptAny tries to decrypt the given data with each of the given publickeys.
// TODO(pmoody): try these in parallel.
func DecryptAny(cipherText []byte, ecPriv *ecdsa.PrivateKey, ecPubs []*ecdsa.PublicKey) ([]byte, error) {
	c := New()
	for _, k := range ecPubs {
		plainText, err := c.Decrypt(cipherText, ecPriv, k)
		if err == nil {
			return plainText, nil
		}
	}
	return nil, errors.New("no public keys decrypt this data")
}

func (c wonkaCrypter) Encrypt(plainText []byte, ecPriv *ecdsa.PrivateKey, ecPub *ecdsa.PublicKey) ([]byte, error) {
	if ecPriv == nil {
		return nil, errors.New("nil private key")
	}

	if ecPub == nil {
		return nil, errors.New("nil public key")
	}

	block, err := SharedSecret(ecPriv, ecPub)
	if err != nil {
		c.log.Error("generating block from shared secret", zap.Error(err))
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	cipherText := append(nonce, aesgcm.Seal(nil, nonce, plainText, nil)...)
	return cipherText, nil
}

func (c wonkaCrypter) Decrypt(cipherText []byte, ecPriv *ecdsa.PrivateKey, ecPub *ecdsa.PublicKey) ([]byte, error) {
	if ecPriv == nil {
		return nil, errors.New("nil private key")
	}

	if ecPub == nil {
		return nil, errors.New("nil public key")
	}

	if textLen := len(cipherText); textLen < aes.BlockSize {
		c.log.Error("ciphertext too short", zap.Any("datalength", textLen))
		return nil, errors.New("ciphertext too short")
	}

	block, err := SharedSecret(ecPriv, ecPub)
	if err != nil {
		c.log.Error("generating block from shared secret", zap.Error(err))
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := cipherText[:aesgcm.NonceSize()]
	cipherText = cipherText[aesgcm.NonceSize():]

	plainText, err := aesgcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, err
	}

	return plainText, nil
}

// golang sometimes chops off leading 0x0 bytes.
func padding(desiredLen int, b *big.Int, sig []byte) []byte {
	for i := sigLen - len(b.Bytes()); i > 0; i-- {
		sig = append(sig, 0x00)
	}
	return append(sig, b.Bytes()...)
}

func (c wonkaCrypter) Sign(data []byte, ecPriv *ecdsa.PrivateKey) ([]byte, error) {
	if ecPriv == nil {
		return nil, errors.New("nil private key")
	}

	h := crypto.SHA256.New()
	h.Write(data)

	r, s, err := ecdsa.Sign(rand.Reader, ecPriv, h.Sum(nil))
	if err != nil {
		c.log.Error("error signing data", zap.Error(err))
		return nil, err
	}

	var sig []byte
	sig = padding(sigLen, r, sig)
	sig = padding(sigLen, s, sig)

	return sig, nil
}

func (c wonkaCrypter) Verify(data, sig []byte, ecPub *ecdsa.PublicKey) bool {
	if ecPub == nil {
		return false
	}

	// TODO(pmoody): see about removing these numeric constants.
	if len(sig) != 64 {
		c.log.Error("invalid signature length",
			zap.Int("length", len(sig)),
			zap.Any("signature", sig),
		)

		return false
	}

	// TODO(pmoody): see about removing these numeric constants.
	r := new(big.Int).SetBytes(sig[0:32])
	s := new(big.Int).SetBytes(sig[32:64])

	// now we have the key, time to verify that sig
	h := crypto.SHA256.New()
	h.Write(data)

	return ecdsa.Verify(ecPub, h.Sum(nil), r, s)
}

// SharedSecret returns secret that's shared between the given
// ecdsa private and public keys.
func SharedSecret(priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) (cipher.Block, error) {
	if priv == nil {
		return nil, errors.New("nil private key")
	}

	if pub == nil {
		return nil, errors.New("nil public key")
	}

	if priv.PublicKey.Curve != pub.Curve {
		return nil, errors.New("bad keys")
	} else if !priv.PublicKey.IsOnCurve(pub.X, pub.Y) {
		return nil, errors.New("bad keys")
	}

	x, y := elliptic.P256().ScalarMult(pub.X, pub.Y, priv.D.Bytes())
	if x.Sign() == 0 && y.Sign() == 0 {
		return nil, errors.New("shared key is at infinity")
	}

	k, err := concatKDF(x.Bytes(), 256)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(k[:32])
	if err != nil {
		return nil, err
	}

	return block, nil
}

func concatKDF(z []byte, kdLen int) ([]byte, error) {
	var s []byte
	hash := crypto.SHA256.New()

	reps := ((kdLen + 7) * 8) / (hash.BlockSize() * 8)
	if big.NewInt(int64(reps)).Cmp(big2To32M1) > 0 {
		return nil, errors.New("key data too long")
	}

	counter := []byte{0, 0, 0, 1}
	var k []byte

	for i := 0; i <= reps; i++ {
		hash.Write(counter)
		hash.Write(z)
		hash.Write(s)
		k = append(k, hash.Sum(nil)...)
		hash.Reset()
		incCounter(counter)
	}

	k = k[:kdLen]
	return k, nil
}

func incCounter(ctr []byte) {
	for i := len(ctr) - 1; i >= 0; i-- {
		if ctr[i]++; ctr[i] != 0 {
			return
		}
	}
}

const _nilCrypterErrorMsg = "wonkaEntityCrypter is nil"

// NewEntityCrypter returns a new EntityCrypter using the provided private key.
//
// If the private key is nil, a nil EntityCrypter is returned.
func NewEntityCrypter(p *ecdsa.PrivateKey) EntityCrypter {
	if p == nil {
		return (*wonkaEntityCrypter)(nil)
	}

	return &wonkaEntityCrypter{
		c: New(),
		p: p,
	}
}

func (e *wonkaEntityCrypter) Encrypt(plainText []byte, ecPub *ecdsa.PublicKey) ([]byte, error) {
	if e == nil {
		return nil, errors.New(_nilCrypterErrorMsg)
	}

	return e.c.Encrypt(plainText, e.p, ecPub)
}

func (e *wonkaEntityCrypter) Decrypt(cipherText []byte, ecPub *ecdsa.PublicKey) ([]byte, error) {
	if e == nil {
		return nil, errors.New(_nilCrypterErrorMsg)
	}

	return e.c.Decrypt(cipherText, e.p, ecPub)
}

func (e *wonkaEntityCrypter) Sign(data []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New(_nilCrypterErrorMsg)
	}

	return e.c.Sign(data, e.p)
}

func (e *wonkaEntityCrypter) Verify(data, sig []byte, ecPub *ecdsa.PublicKey) bool {
	if e == nil {
		return false
	}

	return e.c.Verify(data, sig, ecPub)
}
