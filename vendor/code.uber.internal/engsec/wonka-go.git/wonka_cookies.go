package wonka

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
)

// MarshalCookie turns a Cookie into its wire-format.
func MarshalCookie(c *Cookie) (string, error) {
	b, err := json.Marshal(c)
	if err == nil {
		return base64.StdEncoding.EncodeToString(b), nil
	}

	return "", err
}

// UnmarshalCookie turns the wire-format of a cookie into its struct definition.
func UnmarshalCookie(s string) (*Cookie, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("error decoding cookie")
	}
	var c Cookie
	err = json.Unmarshal(b, &c)
	return &c, err
}

// NewCookie creates a new cookie, destined for `dest`, based on the certificate and key
// associated with the given wonka instance.
func NewCookie(w Wonka, dest string) (*Cookie, error) {
	crypter, ok := w.(Crypter)
	if !ok {
		return nil, errors.New("not a crypter interface")
	}
	cert := crypter.Certificate()

	serial, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, fmt.Errorf("error generating serial: %v", err)
	}

	c := &Cookie{
		Destination: dest,
		Certificate: cert,
		Ctime:       uint64(time.Now().Unix()),
		Serial:      serial.Uint64(),
	}

	toSign, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("error marshalling cookie to sign: %v", err)
	}

	c.Signature, err = crypter.NewEntityCrypter().Sign(toSign)
	if err != nil {
		return nil, fmt.Errorf("error signing cookie: %v", err)
	}

	return c, nil
}

// CheckCookie validates that a given cookie is good.
func CheckCookie(w Wonka, c *Cookie) error {
	if c == nil {
		return errors.New("nil cookie")
	}

	// 1. check that the cookie is meant for us
	if e := w.EntityName(); e != c.Destination {
		return fmt.Errorf("cookie not for me: dest %q, me %q", c.Destination, e)
	}

	// 2. check that the cert private key signed the cookie
	if c.Certificate == nil {
		return errors.New("invalid cookie,empty certificate")
	}

	key, err := c.Certificate.PublicKey()
	if err != nil {
		return fmt.Errorf("error getting key from certificate: %v", err)
	}

	toVerify := *c
	toVerify.Signature = nil
	toVerifyBytes, err := json.Marshal(toVerify)
	if err != nil {
		return fmt.Errorf("error marshalling cookie to verify: %v", err)
	}

	if ok := wonkacrypter.New().Verify(toVerifyBytes, c.Signature, key); !ok {
		return errors.New("cookie signature doesn't verify")
	}

	// 3. check that the cert is valid
	return c.Certificate.CheckCertificate()
}
