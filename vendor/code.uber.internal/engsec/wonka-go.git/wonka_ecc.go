package wonka

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

func pubKeysEq(key1, key2 *ecdsa.PublicKey) bool {
	key1Bytes, err := x509.MarshalPKIXPublicKey(key1)
	if err != nil {
		return false
	}

	key2Bytes, err := x509.MarshalPKIXPublicKey(key2)
	if err != nil {
		return false
	}

	return bytes.Equal(key1Bytes, key2Bytes)
}

// the big.Int math functions in here were heavily, 'inspired' by
// https://github.com/vsergeev/btckeygenie/blob/master/btckey/elliptic.go
// addMod computes z = (x + y) % p.
func addMod(x *big.Int, y *big.Int, p *big.Int) (z *big.Int) {
	z = new(big.Int).Add(x, y)
	z.Mod(z, p)
	return z
}

// subMod computes z = (x - y) % p.
func subMod(x *big.Int, y *big.Int, p *big.Int) (z *big.Int) {
	z = new(big.Int).Sub(x, y)
	z.Mod(z, p)
	return z
}

// expMod computes z = (x^e) % p.
func expMod(x *big.Int, y *big.Int, p *big.Int) (z *big.Int) {
	z = new(big.Int).Exp(x, y, p)
	return z
}

// sqrtMod computes z = sqrt(x) % p.
func sqrtMod(x *big.Int, p *big.Int) (z *big.Int) {
	/* assert that p % 4 == 3 */
	if new(big.Int).Mod(p, big.NewInt(4)).Cmp(big.NewInt(3)) != 0 {
		panic("p is not equal to 3 mod 4!")
	}

	/* z = sqrt(x) % p = x^((p+1)/4) % p */

	e := new(big.Int).Add(p, big.NewInt(1))
	e = e.Rsh(e, 2)

	z = expMod(x, e, p)
	return z
}

// mulMod computes z = (x * y) % p.
func mulMod(x *big.Int, y *big.Int, p *big.Int) (z *big.Int) {
	n := new(big.Int).Set(x)
	z = big.NewInt(0)

	for i := 0; i < y.BitLen(); i++ {
		if y.Bit(i) == 1 {
			z = addMod(z, n, p)
		}
		n = addMod(n, n, p)
	}

	return z
}

// KeyFromCompressed returns the ecdsa.PublicKey from a compressed ec key.
func KeyFromCompressed(compressedKey string) (*ecdsa.PublicKey, error) {
	// go can do funny things with ecc keys, so this is just checking the
	// compressed key is at least long enough to contain the parity prefix so we
	// don't segfault when we grab X.  the IsOnCurve check below verifies that
	// the rest of the key is legit.
	if len(compressedKey) <= 2 {
		return nil, errors.New("key too short")
	}

	curve := elliptic.P256()
	eccX, ok := new(big.Int).SetString(compressedKey[2:], 16)
	if !ok {
		return nil, errors.New("bad compressed key")
	}
	parity := 0x0
	if strings.HasPrefix(compressedKey, "03") {
		parity = 0x1
	}

	p := curve.Params().P
	a := big.NewInt(-3)
	b := curve.Params().B

	// (y^2)%p = (x^3)%p + (ax)%p + %p
	xCubed := expMod(eccX, big.NewInt(3), p)
	axMod := mulMod(a, eccX, p)
	bMod := addMod(b, big.NewInt(0), p)
	rhs := addMod(addMod(xCubed, axMod, p), bMod, p)

	/* y = sqrt(rhs) % p */
	y := sqrtMod(rhs, p)

	/* Use -y if opposite lsb is required */
	if y.Bit(0)&0x1 != uint(parity) {
		y = subMod(big.NewInt(0), y, p)
	}

	if !curve.Params().IsOnCurve(eccX, y) {
		return nil, errors.New("key not on curve")
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     eccX,
		Y:     y,
	}, nil

}

// KeyToCompressed returns the ecc publickey in compressed form.
func KeyToCompressed(x, y *big.Int) string {
	prefix := "03"
	if y.Bit(0) == 0 {
		prefix = "02"
	}

	return fmt.Sprintf("%s%x", prefix, x)
}

// ECCFromRSA turns an RSA private key into a derived ecdsa private key.
func ECCFromRSA(k *rsa.PrivateKey) *ecdsa.PrivateKey {
	// Hash the RSA private key bytes
	h := crypto.SHA256.New()
	h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
	pointKey := h.Sum(nil)

	// Generate a master ECC point derived from the RSA private key
	x, y := elliptic.P256().ScalarBaseMult(pointKey)
	ec := &ecdsa.PrivateKey{
		D: new(big.Int).SetBytes(pointKey),
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     x,
			Y:     y,
		},
	}

	return ec
}
