package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestCSR(t *testing.T) {
	var csrVars = []struct {
		name      string
		badName   bool
		shortName bool
		vb        time.Duration

		err string
	}{
		{name: "foo01-sjc1.prod.uber.internal"},
		{name: "foo01-sjc1.prod.uber.internal", vb: 21 * time.Hour},
		{name: "foo01-sjc1.prod.uber.internal", badName: true,
			err: wonka.BadCertificateSigningRequest},
		{name: "foo01-sjc1.prod.uber.internal", shortName: true},
		{name: "bar.dev.uber.com"},
	}

	for idx, m := range csrVars {
		wonkatestdata.WithUSSHHostAgent(m.name, func(agentPath string, caKey ssh.PublicKey) {
			wonkatestdata.WithWonkaMaster(m.name, func(r common.Router, handlerCfg common.HandlerConfig) {
				defer testhelper.SetEnvVar("WONKA_USSH_HOST_CA",
					fmt.Sprintf("@cert-authority * %s", ssh.MarshalAuthorizedKey(caKey)))()
				SetupHandlers(r, handlerCfg)
				require.NotNil(t, r, "%d setup handlers returned nil", idx)
				a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
				require.NoError(t, err, "%d, ssh-agent dial error: %v", idx, err)

				w, err := wonka.Init(wonka.Config{EntityName: m.name, Agent: agent.NewClient(a)})
				require.NoError(t, err, "%d, error initializing wonka: %v", idx, err)

				cert, _, err := wonka.NewCertificate(wonka.CertEntityName(m.name), wonka.CertHostname(m.name))
				require.NoError(t, err, "%d, error generating a cert: %v", idx, err)
				cert.ValidBefore = uint64(time.Now().Add(time.Minute).Unix())

				if m.badName {
					cert.Host = cert.Host + "bad"
				}
				if m.shortName {
					cert.Host = strings.Split(cert.Host, ".")[0]
				}

				if m.vb != 0 {
					cert.ValidBefore = uint64(time.Now().Add(m.vb).Unix())
				}

				err = w.CertificateSignRequest(context.Background(), cert, nil)
				if m.err != "" {
					require.Error(t, err, "should error")
					require.Contains(t, err.Error(), m.err, "%d, should contain %s", idx, m.err)
				} else {
					require.NoError(t, err, "%d, signing error: %v", idx, err)
				}

				if m.vb != 0 {
					certVB := time.Unix(int64(cert.ValidBefore), 0)
					vb := time.Now().Add(m.vb).Add(-time.Minute)
					require.True(t, certVB.Before(vb), "time should've been replaced")
				}
			})
		})
	}
}

func newTestSigningCert(t *testing.T, wonkaPriv ecdsa.PrivateKey) (*wonka.Certificate, *ecdsa.PrivateKey) {
	signingCert, signingKey, err := wonka.NewCertificate(wonka.CertEntityName("foo"), wonka.CertHostname("host"))
	require.NoError(t, err)

	signingCertBytes, err := wonka.MarshalCertificate(*signingCert)
	require.NoError(t, err)

	signingCert.Signature, err = wonkacrypter.New().Sign(signingCertBytes, &wonkaPriv)
	require.NoError(t, err)

	err = signingCert.CheckCertificate()
	require.NoError(t, err, "signing cert doesn't verify: %v", err)
	return signingCert, signingKey
}
func TestRefreshCert(t *testing.T) {
	wonkaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating key: %v", err)

	oldWonkaKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaPriv.PublicKey}
	defer func() {
		wonka.WonkaMasterPublicKeys = oldWonkaKeys
	}()

	signingCert, signingKey := newTestSigningCert(t, *wonkaPriv)
	signingCertBytes, err := wonka.MarshalCertificate(*signingCert)
	require.NoError(t, err)

	certToSign, _, err := wonka.NewCertificate(
		wonka.CertEntityName(signingCert.EntityName),
		wonka.CertHostname(signingCert.Host),
		wonka.CertTaskIDTag(signingCert.Tags[wonka.TagTaskID]))
	require.NoError(t, err)

	certBytes, err := wonka.MarshalCertificate(*certToSign)
	require.NoError(t, err)

	csr := wonka.CertificateSigningRequest{
		Certificate:        certBytes,
		SigningCertificate: signingCertBytes,
	}

	toSign, err := json.Marshal(csr)
	require.NoError(t, err)

	csr.Signature, err = wonkacrypter.New().Sign(toSign, signingKey)
	require.NoError(t, err)

	h := csrHandler{
		eccPrivateKey: wonkaPriv,
		log:           zap.L(),
	}

	// everything above here is setup.
	// TODO(pmoody): move this setup code into a helper that can be used by other tests.
	// this doesn't actually sign the new cert, it just verifies the request is good
	err = h.existingCertVerify(csr, certToSign, signingCert)
	require.NoError(t, err)
}

func signTestCertificateRequest(t *testing.T,
	req *wonka.CertificateSigningRequest,
	signingKey *ecdsa.PrivateKey,
) {
	toSign, err := json.Marshal(req)
	require.NoError(t, err)

	req.Signature, err = wonkacrypter.New().Sign(toSign, signingKey)
	require.NoError(t, err)
}

func serializeCertificateSigningRequestAndMakeCall(t *testing.T,
	w *httptest.ResponseRecorder,
	h xhttp.Handler,
	req wonka.CertificateSigningRequest) {
	data, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal csr: %v", err)
	r := &http.Request{Body: ioutil.NopCloser(bytes.NewReader(data))}
	h.ServeHTTP(context.Background(), w, r)
}

func TestCSRHandlerUnits(t *testing.T) {
	validHandler := newCSRHandler(getTestConfig(t))

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
			Message:    wonka.BadCertificateSigningRequest,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				req := wonka.CertificateSigningRequest{
					Certificate: []byte("Snozzberries? Who ever heard of a snozzberry?"),
				}
				serializeCertificateSigningRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when certificate is present but is not actually enrolled",
			statusCode: http.StatusForbidden,
			Message:    wonka.BadCertificateSigningRequest,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				cert := wonka.Certificate{}
				c, err := wonka.MarshalCertificate(cert)
				require.NoError(t, err, "failed to marshal cert")
				req := wonka.CertificateSigningRequest{
					Certificate: c,
				}
				serializeCertificateSigningRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when signing cert is present but invalid",
			statusCode: http.StatusForbidden,
			Message:    wonka.BadCertificateSigningRequest,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {
				cert := wonka.Certificate{}
				c, err := wonka.MarshalCertificate(cert)
				require.NoError(t, err, "failed to marshal cert")
				req := wonka.CertificateSigningRequest{
					Certificate:        c,
					SigningCertificate: []byte("invalid"),
				}
				serializeCertificateSigningRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when signing cert is valid but is outside the time window ",
			statusCode: http.StatusBadRequest,
			Message:    wonka.ErrTimeWindow,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {

				wonkaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err, "error generating key: %v", err)

				oldWonkaKeys := wonka.WonkaMasterPublicKeys
				wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaPriv.PublicKey}
				defer func() {
					wonka.WonkaMasterPublicKeys = oldWonkaKeys
				}()

				signingCert, signingKey := newTestSigningCert(t, *wonkaPriv)
				signingCertBytes, err := wonka.MarshalCertificate(*signingCert)
				require.NoError(t, err)

				cert := wonka.Certificate{
					EntityName: signingCert.EntityName,
					Host:       signingCert.Host,
				}
				c, err := wonka.MarshalCertificate(cert)
				require.NoError(t, err, "failed to marshal cert")
				req := wonka.CertificateSigningRequest{
					Certificate:        c,
					SigningCertificate: signingCertBytes,
				}
				signTestCertificateRequest(t, &req, signingKey)
				serializeCertificateSigningRequestAndMakeCall(t, w, validHandler, req)
			},
		},
		{
			name:       "errors when signing cert is valid but is after the time window ",
			statusCode: http.StatusBadRequest,
			Message:    wonka.CSRExpired,
			makeCall: func(t *testing.T, w *httptest.ResponseRecorder) {

				wonkaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err, "error generating key: %v", err)

				oldWonkaKeys := wonka.WonkaMasterPublicKeys
				wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&wonkaPriv.PublicKey}
				defer func() {
					wonka.WonkaMasterPublicKeys = oldWonkaKeys
				}()

				signingCert, signingKey := newTestSigningCert(t, *wonkaPriv)
				signingCertBytes, err := wonka.MarshalCertificate(*signingCert)
				require.NoError(t, err)

				cert := wonka.Certificate{
					EntityName: signingCert.EntityName,
					Host:       signingCert.Host,
					ValidAfter: uint64(time.Now().Unix()),
				}
				c, err := wonka.MarshalCertificate(cert)
				require.NoError(t, err, "failed to marshal cert")
				req := wonka.CertificateSigningRequest{
					Certificate:        c,
					SigningCertificate: signingCertBytes,
				}
				signTestCertificateRequest(t, &req, signingKey)
				serializeCertificateSigningRequestAndMakeCall(t, w, validHandler, req)
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

func TestLaunchRequest(t *testing.T) {
	var testVars = []struct {
		name       string
		badLRSig   bool
		badCertSig bool
		empty      bool
		badDecode  bool
		badJSON    bool

		errMsg string
	}{
		{name: "foo"},
		{name: "foo", badLRSig: true, errMsg: "error verifying launch request"},
		{name: "foo", badCertSig: true, errMsg: "error verifying launch request"},
		{name: "foo", empty: true, errMsg: "no launch request included"},
		{name: "foo", badDecode: true, errMsg: "error decoding signed launch request"},
		{name: "foo", badJSON: true, errMsg: "error unmarshalling signed launch request"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			withSignedCert(m.name, func(cert *wonka.Certificate, privKey *ecdsa.PrivateKey) {
				if m.badCertSig {
					cert.Signature = nil
				}

				toSign, err := json.Marshal(wonka.LaunchRequest{SvcID: m.name})
				require.NoError(t, err)

				certSig, err := wonka.NewCertificateSignature(*cert, privKey, toSign)
				require.NoError(t, err)

				if m.badLRSig {
					certSig.Signature = nil
				}

				certSigBytes, err := json.Marshal(certSig)
				require.NoError(t, err)

				certSigStr := base64.StdEncoding.EncodeToString(certSigBytes)

				if m.empty {
					certSigStr = ""
				}

				if m.badDecode {
					certSigStr = "foo"
				}

				if m.badJSON {
					certSigStr = base64.StdEncoding.EncodeToString([]byte("foo"))
				}

				lr, err := verifyLaunchRequest(certSigStr)
				if m.errMsg == "" {
					require.NoError(t, err)
					require.Equal(t, m.name, lr.SvcID)
				} else {
					require.Error(t, err)
					require.Contains(t, err.Error(), m.errMsg)
				}
			})
		})
	}
}

func TestCertGrantingCert(t *testing.T) {
	var testVars = []struct {
		name         string
		badSSHSig    bool
		noUsshCert   bool
		noUsshVerify bool
		noLR         bool
		badWMKey     bool

		errMsg string
	}{
		{name: "foo"},
		{name: "foo", noUsshCert: true, errMsg: "ssh: no key found"},
		{name: "foo", noUsshVerify: true, errMsg: "error validating signing ussh certificate"},
		{name: "foo", badSSHSig: true, errMsg: "error unmarshalling ssh signature"},

		{name: "foo", noLR: true, errMsg: "no launch request included"},
		{name: "foo", badWMKey: true, errMsg: "error verifying launch request"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			// this signs the original launch request, it's like mesos-master
			withSignedLaunchRequest(func(certSig []byte, wmKeys []*ecdsa.PublicKey) {
				csrStr := base64.StdEncoding.EncodeToString(certSig)
				if m.noLR {
					csrStr = ""
				}

				// this mocks out the host ussh cert that signs the cgc
				withUsshAgent(func(a agent.Agent, usshKey ssh.PublicKey) {
					usshCert := string(ssh.MarshalAuthorizedKey(usshKey))

					if m.noUsshCert {
						usshCert = ""
					}

					// this is our cgc
					withSignedCert(m.name, func(signingCert *wonka.Certificate, signingKey *ecdsa.PrivateKey) {
						// first, sign our new cgc with the ussh key
						var err error
						if !m.badSSHSig {
							signingCert, err = replaceSignature(a, usshKey, signingCert)
							require.NoError(t, err)
						}

						// now generate a new cert and key
						cert, csr := newCertAndCSR(m.name, signingCert, signingKey)

						// this tests the cgc works
						h := csrHandler{
							usshHostKeyCallback: func(h string, r net.Addr, k ssh.PublicKey) error {
								if m.noUsshVerify {
									return errors.New("doesn't verify")
								}
								return nil
							},
						}

						// make sure the wonkamaster public key validates the original launch request
						if !m.badWMKey {
							wonka.WonkaMasterPublicKeys = wmKeys
						}

						// now test
						err = h.cgCertVerify(csr, cert, signingCert)
						if m.errMsg != "" {
							require.Error(t, err)
							require.Contains(t, err.Error(), m.errMsg)
						} else {
							require.NoError(t, err)
						}
					}, wonka.CertLaunchRequestTag(csrStr), wonka.CertUSSHCertTag(usshCert))
				})
			})
		})
	}
}

func TestUSSHHostVerify(t *testing.T) {
	h := csrHandler{
		usshHostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		log: zap.NewNop(),
	}
	const _host = "some_host"
	const _task = "fake_task"

	rejectEntity := func(t *testing.T, e string, certType wonka.EntityType, exErr string) {
		withUsshAgent(func(a agent.Agent, pub ssh.PublicKey) {
			usshCert := ssh.Certificate{
				Key: pub,
			}

			certToSign, _, err := wonka.NewCertificate(
				wonka.CertEntityName(e),
				wonka.CertHostname(_host),
				wonka.CertTaskIDTag(_task),
				wonka.CertEntityType(certType))
			require.NoError(t, err)
			require.NotNil(t, certToSign)

			certBytes, err := wonka.MarshalCertificate(*certToSign)
			require.NoError(t, err)

			csr := wonka.CertificateSigningRequest{
				Certificate:     certBytes,
				USSHCertificate: ssh.MarshalAuthorizedKey(pub),
			}

			err = h.usshHostVerify(csr, &usshCert, certToSign)
			require.EqualError(t, err, exErr, "expected to reject entity %q", e)
		})
	}

	var testCases = []struct {
		entityName string
		entityType wonka.EntityType
		err        string
	}{
		{
			"fake@uber.com",
			wonka.EntityTypeUser,
			`cannot validate a wonka "User" cert with a USSH host cert`,
		},
		{
			"fake@uber.com",
			wonka.EntityTypeService,
			`invalid entity name for USSH host cert validation: "fake@uber.com"`,
		},
		{
			"fake@uber.com  ",
			wonka.EntityTypeService,
			`invalid entity name for USSH host cert validation: "fake@uber.com  "`,
		},
		{
			"ad:engineering",
			wonka.EntityTypeService,
			`invalid entity name for USSH host cert validation: "ad:engineering"`,
		},
		{
			"AD:engineering",
			wonka.EntityTypeService,
			`invalid entity name for USSH host cert validation: "AD:engineering"`,
		},
	}

	for _, tt := range testCases {
		t.Run(fmt.Sprintf("%s_%s", tt.entityName, tt.entityType), func(t *testing.T) {
			rejectEntity(t, tt.entityName, tt.entityType, tt.err)
		})
	}
}

func newCertAndCSR(name string, signingCert *wonka.Certificate, signingKey *ecdsa.PrivateKey) (*wonka.Certificate, wonka.CertificateSigningRequest) {
	cert, _, err := wonka.NewCertificate(wonka.CertEntityName(name), wonka.CertHostname(name))
	if err != nil {
		panic(err)
	}

	certBytes, err := wonka.MarshalCertificate(*cert)
	if err != nil {
		panic(err)
	}

	signingCertBytes, err := wonka.MarshalCertificate(*signingCert)
	if err != nil {
		panic(err)
	}

	csr := wonka.CertificateSigningRequest{
		Certificate:        certBytes,
		SigningCertificate: signingCertBytes,
	}

	toSign, err := json.Marshal(csr)
	if err != nil {
		panic(err)
	}

	// now sign the new csr with our cgc signing key
	csr.Signature, err = wonkacrypter.New().Sign(toSign, signingKey)
	if err != nil {
		panic(err)
	}

	return cert, csr
}

func replaceSignature(a agent.Agent, sshKey ssh.PublicKey, cert *wonka.Certificate) (*wonka.Certificate, error) {
	cert.Signature = nil
	toSign, err := json.Marshal(cert)
	if err != nil {
		return nil, err
	}

	sshSig, err := a.Sign(sshKey, toSign)
	if err != nil {
		return nil, err
	}
	cert.Signature = ssh.Marshal(sshSig)

	return cert, nil
}

func withUsshAgent(fn func(agent.Agent, ssh.PublicKey)) {
	wonkatestdata.WithUSSHHostAgent("foo", func(agentPath string, _ ssh.PublicKey) {
		agentSock, err := net.Dial("unix", agentPath)
		if err != nil {
			panic(err)
		}

		a := agent.NewClient(agentSock)
		keys, err := a.List()
		if err != nil {
			panic(err)
		}
		if len(keys) != 1 {
			panic(fmt.Sprintf("expected 1 key, got %d", len(keys)))
		}

		usshKey, err := ssh.ParsePublicKey(keys[0].Blob)
		if err != nil {
			panic(err)
		}

		fn(a, usshKey)
	})
}

func withSignedLaunchRequest(fn func([]byte, []*ecdsa.PublicKey)) {
	withSignedCert("foo", func(origCert *wonka.Certificate, origKey *ecdsa.PrivateKey) {
		lr := wonka.LaunchRequest{
			Hostname: "foo",
			SvcID:    "foo",
			TaskID:   "foo",
		}
		lrBytes, err := json.Marshal(lr)
		if err != nil {
			panic(err)
		}
		certSig, err := wonka.NewCertificateSignature(*origCert, origKey, lrBytes)
		if err != nil {
			panic(err)
		}
		certSigBytes, err := json.Marshal(certSig)
		if err != nil {
			panic(err)
		}

		fn(certSigBytes, wonka.WonkaMasterPublicKeys)
	})
}
func withSignedCert(name string, fn func(*wonka.Certificate, *ecdsa.PrivateKey), opts ...wonka.CertificateOption) {
	cert, privKey, err := wonka.NewCertificate(wonka.CertEntityName(name), wonka.CertHostname(name))
	if err != nil {
		panic(err)
	}

	for _, o := range opts {
		o(cert)
	}

	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	oldKeys := wonka.WonkaMasterPublicKeys
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&signer.PublicKey}
	defer func() { wonka.WonkaMasterPublicKeys = oldKeys }()

	err = cert.SignCertificate(signer)
	if err != nil {
		panic(err)
	}

	fn(cert, privKey)
}
