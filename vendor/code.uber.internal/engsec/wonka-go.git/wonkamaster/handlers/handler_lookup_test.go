package handlers

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"path"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	. "code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var (
	keySize = 4096
)

var lookupVars = []struct {
	e1 string
	e2 string

	result string // error result
	entity string // successful lookup
}{
	{e1: "wonkaSample:foober", e2: "wonkaSample:doober", entity: "doober"},
	{e1: "wonkaSample:foober", result: wonka.EntityUnknown},
}

func TestLookupEntity(t *testing.T) {
	log := zap.S()

	for idx, m := range lookupVars {
		WithWonkaMaster(m.e1, func(r common.Router, handlerCfg common.HandlerConfig) {
			SetupHandlers(r, handlerCfg)
			WithTempDir(func(dir string) {
				pubPath := path.Join(dir, "public.pem")
				privPath := path.Join(dir, "private.pem")
				ctx := context.TODO()

				e := generateKey(pubPath, privPath)
				require.NoError(t, e, "generating keys")

				privateKey := hashes(privPath)
				log.Infof("generated priv %s, pub %s",
					keys.KeyHash(privateKey), keys.KeyHash(&privateKey.PublicKey))

				ecc := crypto.SHA256.New()
				ecc.Write([]byte(x509.MarshalPKCS1PrivateKey(privateKey)))
				e1 := wonka.Entity{
					EntityName:   m.e1,
					PublicKey:    keys.RSAPemBytes(&privateKey.PublicKey),
					ECCPublicKey: wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(ecc.Sum(nil))),
				}

				err := handlerCfg.DB.Create(ctx, &e1)
				require.NoError(t, err, "test %d create should succeed", idx)

				if m.e2 != "" {
					e2 := wonka.Entity{
						EntityName:   m.e2,
						PublicKey:    keys.RSAPemBytes(&privateKey.PublicKey),
						ECCPublicKey: wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(ecc.Sum(nil))),
					}
					err = handlerCfg.DB.Create(ctx, &e2)
					require.NoError(t, err, "test %d create should succeed", idx)
				}

				cfg := wonka.Config{
					EntityName:     m.e1,
					EntityLocation: "none",
					PrivateKeyPath: privPath,
				}

				w, e := wonka.Init(cfg)
				require.NoError(t, e, "%d, init: %v", idx, e)

				entity, e := w.Lookup(ctx, m.e2)
				if m.result != "" {
					require.Error(t, e, "%d lookup should fail", idx)
					require.Contains(t, e.Error(), m.result, "test %d", idx)
				} else {
					require.NoError(t, e, "lookup: %v, %d", e, idx)
					require.Equal(t, m.e2, entity.EntityName, "test %d", idx)
				}
			})
		})
	}
}

var timeVars = []struct {
	ctime   int
	goodFor time.Duration
	errMsg  string
}{{ctime: 0, goodFor: time.Minute},
	{ctime: -70, goodFor: time.Minute, errMsg: "expired ctime"},
}

func TestValidTime(t *testing.T) {
	for idx, m := range timeVars {
		cTime := time.Now().Add(time.Duration(m.ctime) * time.Second)
		err := validTime(int(cTime.Unix()), m.goodFor)
		if m.errMsg != "" {
			require.Error(t, err, "test %d, should error with %s", idx, m.errMsg)
			require.Contains(t, err.Error(), m.errMsg)
		} else {
			require.NoError(t, err, "test %d, err: %v", idx, err)
		}
	}
}

func generateKey(pubPath, privPath string) error {
	log := zap.S()

	k := PrivateKey()
	log.Infof("generate key %s, %s", keys.KeyHash(k), keys.KeyHash(&k.PublicKey))

	b := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}

	if e := ioutil.WriteFile(privPath, pem.EncodeToMemory(&b),
		0644); e != nil {
		return e
	}
	pubBytes, e := x509.MarshalPKIXPublicKey(k.Public())
	if e != nil {
		return e
	}
	pub := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}

	return ioutil.WriteFile(pubPath, pem.EncodeToMemory(&pub), 0644)
}

func hashes(priv string) *rsa.PrivateKey {
	b, e := ioutil.ReadFile(priv)
	if e != nil {
		panic(e)
	}

	p, _ := pem.Decode(b)
	if p == nil {
		panic("no p")
	}

	k, e := x509.ParsePKCS1PrivateKey(p.Bytes)
	if e != nil {
		panic(e)
	}

	return k
}
