package handlers

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
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
		wonkatestdata.WithWonkaMaster(m.e1, func(r common.Router, handlerCfg common.HandlerConfig) {
			SetupHandlers(r, handlerCfg)
			wonkatestdata.WithTempDir(func(dir string) {
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

func TestVerifyLookupSignature(t *testing.T) {
	var testVars = []struct {
		name           string
		version        string
		emptySignature bool
		badSignature   bool

		errMsg string
	}{
		{name: "good_sig_everything", version: wonka.SignEverythingVersion},
		{name: "bad_sig_everything", version: wonka.SignEverythingVersion, badSignature: true,
			errMsg: "ec signature check failed"},
		{name: "bad_empty_sig_everything", version: wonka.SignEverythingVersion, emptySignature: true,
			errMsg: "empty signature"},
		{name: "good_old"},
		{name: "bad_old_empty_sig", emptySignature: true, errMsg: "empty signature"},
		{name: "bad_old_bad_signature", badSignature: true, errMsg: "claim signature check failed"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d_%s", idx, m.name), func(t *testing.T) {
			req := wonka.LookupRequest{
				Version:         m.version,
				Ctime:           int(time.Now().Unix()),
				EntityName:      "foober",
				RequestedEntity: "doober",
				SigType:         wonka.SHA256,
			}

			var pubKey crypto.PublicKey
			var err error
			var sig []byte
			if req.Version == wonka.SignEverythingVersion {
				privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err)
				pubKey = &privKey.PublicKey

				toSign, err := json.Marshal(req)
				require.NoError(t, err)
				sig, err = wonkacrypter.New().Sign(toSign, privKey)
				require.NoError(t, err)

			} else {
				privKey, err := rsa.GenerateKey(rand.Reader, 1024)
				require.NoError(t, err)
				pubKey = &privKey.PublicKey

				toSign := []byte(fmt.Sprintf("%s<%d>%s", req.EntityName, req.Ctime, req.RequestedEntity))
				h := crypto.SHA256.New()
				h.Write(toSign)
				sig, err = privKey.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
				require.NoError(t, err)
			}

			if m.badSignature {
				sig = []byte("foober")
			}

			if !m.emptySignature {
				req.Signature = base64.StdEncoding.EncodeToString(sig)
			}

			h := lookupHandler{log: zap.L()}
			err = h.verifyLookupSignature(pubKey, req)
			if m.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), m.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVerifyUSSHLookupSignature(t *testing.T) {
	var testVars = []struct {
		name         string
		version      string
		badSignature bool

		errMsg string
	}{
		{name: "good_sign_everything", version: wonka.SignEverythingVersion},
		{name: "good_old"},
		{name: "bad_sign_everything", version: wonka.SignEverythingVersion, badSignature: true,
			errMsg: wonka.LookupInvalidUSSHSignature},
		{name: "bad_old", badSignature: true, errMsg: wonka.LookupInvalidUSSHSignature},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d_%s", idx, m.name), func(t *testing.T) {
			cert, publicKey, a := newUSSHCert("foober")

			req := wonka.LookupRequest{
				Version:         m.version,
				Ctime:           int(time.Now().Unix()),
				EntityName:      "foober",
				RequestedEntity: "doober",
				USSHCertificate: string(ssh.MarshalAuthorizedKey(cert)),
			}

			var toSign []byte
			var err error
			if req.Version == wonka.SignEverythingVersion {
				toSign, err = json.Marshal(req)
				require.NoError(t, err)
			} else {
				toSign = []byte(fmt.Sprintf("%s<%d>%s|%s", req.EntityName, req.Ctime, req.RequestedEntity,
					req.USSHCertificate))
			}

			if m.badSignature {
				toSign = []byte("foober")
			}

			sig, err := a.Sign(cert, toSign)
			require.NoError(t, err)

			req.USSHSignature = base64.StdEncoding.EncodeToString(sig.Blob)
			req.USSHSignatureType = sig.Format

			h := lookupHandler{log: zap.L(), usshCAKeys: []ssh.PublicKey{publicKey}}
			err = h.verifyUsshSignature(req)
			if m.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), m.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func newUSSHCert(name string) (*ssh.Certificate, ssh.PublicKey, agent.Agent) {
	signerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	signer, err := ssh.NewSignerFromKey(signerKey)
	if err != nil {
		panic(err)
	}

	signerPublicKey, err := ssh.NewPublicKey(&signerKey.PublicKey)
	if err != nil {
		panic(err)
	}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		panic(err)
	}

	cert := &ssh.Certificate{
		Serial:          1,
		Key:             pubKey,
		ValidPrincipals: []string{name},
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
		ValidAfter:      uint64(time.Now().Add(-time.Minute).Unix()),
		CertType:        ssh.UserCert,
	}

	err = cert.SignCert(rand.Reader, signer)
	if err != nil {
		panic(err)
	}

	a := agent.NewKeyring()
	err = a.Add(agent.AddedKey{PrivateKey: privKey, Certificate: cert})
	if err != nil {
		panic(err)
	}

	return cert, signerPublicKey, a
}

func generateKey(pubPath, privPath string) error {
	log := zap.S()

	k := wonkatestdata.PrivateKey()
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

func serializeLookupRequestAndMakeCall(t *testing.T,
	w *httptest.ResponseRecorder,
	h xhttp.Handler,
	req wonka.LookupRequest) {
	data, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal resolve request: %v", err)
	r := &http.Request{Body: ioutil.NopCloser(bytes.NewReader(data))}
	h.ServeHTTP(context.Background(), w, r)
}

func TestLookupHandlerUnits(t *testing.T) {
	validHandler := newLookupHandler(getTestConfig(t))

	var testCases = []struct {
		name       string
		statusCode int
		Message    string
		makeCall   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "errors when request is invalid json",
			statusCode: http.StatusBadRequest,
			Message:    wonka.DecodeError,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				r := &http.Request{Body: ioutil.NopCloser(
					bytes.NewReader([]byte("Snozzberries? Who ever heard of a snozzberry?")))}
				validHandler.ServeHTTP(context.Background(), w, r)
			},
		},
		{
			name:       "errors when certificate is present but can't be deserialized",
			statusCode: http.StatusBadRequest,
			Message:    wonka.LookupInvalidSignature,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.LookupRequest{
					Certificate: []byte("Snozzberries? Who ever heard of a snozzberry?"),
				}
				serializeLookupRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when certificate is present but the signature can't be deserialized",
			statusCode: http.StatusBadRequest,
			Message:    wonka.LookupInvalidSignature,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.LookupRequest{
					Certificate: []byte("Snozzberries? Who ever heard of a snozzberry?"),
					Signature:   "John Hanncock",
				}
				serializeLookupRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when ussh certificate is present but can't be deserialized",
			statusCode: http.StatusBadRequest,
			Message:    wonka.ResultRejected,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.LookupRequest{
					USSHSignature:   "Snozzberries? Who ever heard of a snozzberry?",
					USSHCertificate: "Snozzberries? Who ever heard of a snozzberry?",
				}
				serializeLookupRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when entitity is not enrolled",
			statusCode: http.StatusBadRequest,
			Message:    wonka.ResultRejected,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.LookupRequest{}
				serializeLookupRequestAndMakeCall(t, w, validHandler, req)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.makeCall(t, w)
			resp := w.Result()
			decoder := json.NewDecoder(resp.Body)
			var m wonka.GenericResponse
			err := decoder.Decode(&m)
			assert.NoError(t, err, "err was not nil")
			assert.Equal(t, tc.statusCode, resp.StatusCode, "status code did not match expected")
			assert.Equal(t, tc.Message, m.Result, "error message did not match expected")
		})
	}
}
