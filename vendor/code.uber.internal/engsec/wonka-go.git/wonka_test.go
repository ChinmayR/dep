package wonka_test

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
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	. "code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"gopkg.in/yaml.v2"
)

var initVars = []struct {
	name string
	dir  string
	key  string

	keyType   wonka.KeyType
	errString string
}{
	{errString: wonka.ErrNoEntity.Error()},
	{name: "foo", dir: "foo", key: "local"},
	{name: "foo", dir: "foo", key: "langley"},
	{name: "foo", dir: "foo", key: "env"},
	{name: "foo", dir: "foo", key: "rsa"},
	{name: "foo", dir: "foo", errString: "load keys failed"},
}

// TestMain function runs onces, before all tests.
func TestMain(m *testing.M) {
	tracer := mocktracer.New()
	// wonkamaster still uses the global tracer.
	opentracing.InitGlobalTracer(tracer)
	code := m.Run()
	os.Exit(code)
}

func TestInit(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	for idx, m := range initVars {
		t.Run(fmt.Sprintf("test_%d_%s", idx, m.key), func(t *testing.T) {
			wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
				WithTempDir(func(dir string) {
					handlers.SetupHandlers(r, handlerCfg)
					defer testhelper.UnsetEnvVar("SSH_AUTH_SOCK")()
					cfg := wonka.Config{
						EntityName: m.name,
					}
					ctx := context.Background()

					switch m.key {
					case "local":
						k := PrivateKeyFromPem(RSAPrivKey)
						privKeyPath := path.Join(dir, "wonka_private.pem")
						e := WritePrivateKey(k, privKeyPath)
						require.NoError(t, e, "writing privkey")
						cfg.PrivateKeyPath = privKeyPath
						EnrollEntity(ctx, t, handlerCfg.DB, m.name, k)
					case "langley":
						require.NotEmpty(t, m.name, "can't have a blank service name")
						p := path.Join(dir, m.name)
						e := os.Mkdir(p, 0755)
						require.NoError(t, e, "making service directory: %v", e)

						k := wonka.SecretsYAML{
							WonkaPrivate: strings.Replace(RSAPrivKey, string('\n'), "", -1),
						}

						lBytes, err := yaml.Marshal(k)
						require.NoError(t, err, "yaml should marshal: %v", e)
						privKeyPath := path.Join(p, "wonka_private.yaml")
						err = ioutil.WriteFile(privKeyPath, lBytes, 0444)
						require.NoError(t, err, "writing private key shouldn't fail: %v", e)
						cfg.PrivateKeyPath = privKeyPath
						EnrollEntity(ctx, t, handlerCfg.DB, m.name, PrivateKeyFromPem(RSAPrivKey))
					case "env":
						p := path.Join(dir, m.name)
						e := os.Mkdir(p, 0755)
						require.NoError(t, e, "making service directory: %v", e)

						cert, privKey, err := wonka.NewCertificate(wonka.CertEntityName(m.name), wonka.CertHostname(m.name))
						require.NoError(t, err, "generating certificate shouldn't fail: %v", err)

						certBytes, err := wonka.MarshalCertificate(*cert)
						require.NoError(t, err, "error marshalling certificate: %v", err)
						certPath := path.Join(p, "wonka_certificate")
						err = ioutil.WriteFile(certPath, certBytes, 0444)
						require.NoError(t, err, "writing private key shouldn't fail: %v", e)
						defer testhelper.SetEnvVar("WONKA_CLIENT_CERT", certPath)()

						ecBytes, err := x509.MarshalECPrivateKey(privKey)
						require.NoError(t, err, "error marshalling privkey: %v", err)
						eccKeyPath := path.Join(p, "wonka_ecc_key")
						err = ioutil.WriteFile(eccKeyPath, []byte(base64.StdEncoding.EncodeToString(ecBytes)), 0444)
						require.NoError(t, err, "writing private key shouldn't fail: %v", e)
						defer testhelper.SetEnvVar("WONKA_CLIENT_KEY", eccKeyPath)()
					case "rsa":
						cfg.PrivateKeyPath = RSAPrivKey
						EnrollEntity(ctx, t, handlerCfg.DB, m.name, PrivateKeyFromPem(RSAPrivKey))
					default:
						// do nothing, to trigger terminal error case
					}

					_, err := wonka.Init(cfg)
					if m.errString != "" {
						require.Error(t, err, "init should fail")
						require.Contains(t, err.Error(), m.errString, "wrong error returned")
					} else {
						require.NoError(t, err, "init should succeed")
					}
				})
			})
		})
	}
}

func TestPrivKeyPathDoesNotExist(t *testing.T) {
	if testhelper.IsProductionEnvironment {
		t.Skip()
	}

	const _expectedError = "error reading private key from file: open ./this-is-not-a-file.yaml: no such file or directory"
	wonkatestdata.WithWonkaMaster("", func(r common.Router, cfg common.HandlerConfig) {
		WithTempDir(func(dir string) {
			defer testhelper.UnsetEnvVar("SSH_AUTH_SOCK")()
			ctx := context.Background()

			cfg := wonka.Config{
				EntityName:     "TestPrivKeyPathDoesNotExist",
				PrivateKeyPath: "./this-is-not-a-file.yaml",
			}
			_, err := wonka.InitWithContext(ctx, cfg)
			require.Error(t, err, "initializing wonka should fail")
			assert.Contains(t, err.Error(), "load keys failed:", "wrong error returned")
			assert.Contains(t, err.Error(), _expectedError, "wrong error returned")
		})
	})

}

func TestWonkaMasterECCKey(t *testing.T) {
	var keyVars = []struct {
		compressed    bool
		badCompressed bool

		err    bool
		errMsg string
	}{
		{err: false},
		{compressed: true},
		{compressed: true, badCompressed: true, err: true, errMsg: ""},
	}

	for idx, m := range keyVars {
		defer testhelper.SetEnvVar("WONKA_MASTER_ECC_PUB", wonka.ECCPUB)()

		if m.compressed && m.badCompressed {
			defer testhelper.SetEnvVar("WONKA_MASTER_ECC_PUB", "foobar")()
		}

		err := wonka.InitWonkaMasterECC()
		if m.err {
			require.Error(t, err, "test %d, bad key should error", idx)
			require.Contains(t, err.Error(), m.errMsg, "test %d")
		} else {
			require.NoError(t, err, "test %d, good key shouldn't error: %v", idx, err)
		}
	}
}

var enrollVars = []struct {
	name     string
	toEnroll string
	location string
	requires string

	location2 string
	requires2 string

	initErr   bool
	enrollErr bool
	errMsg    string
}{
	{name: "user@uber.com", toEnroll: "servicefoo"},
	{initErr: true, errMsg: "no entity name provided"},
	//{name: "user@uber.com", toElocation: "place"},
	//{name: "wonkaSample:test"},
}

func TestEnrollEntity(t *testing.T) {
	for idx, m := range enrollVars {
		WithTempDir(func(dir string) {
			k := PrivateKeyFromPem(RSAPrivKey)
			privKeyPath := path.Join(dir, "wonka_private.pem")
			e := WritePrivateKey(k, privKeyPath)
			require.NoError(t, e, "writing privkey")

			wonkatestdata.WithWonkaMaster(m.name, func(r common.Router, handlerCfg common.HandlerConfig) {
				wonkatestdata.WithUSSHAgent(m.name, func(agentPath string, caKey ssh.PublicKey) {
					agentSock, err := net.Dial("unix", agentPath)
					require.NoError(t, err, "connecting to agent")
					a := agent.NewClient(agentSock)

					handlerCfg.Ussh = []ssh.PublicKey{caKey}
					mem := make(map[string][]string, 0)
					mem[m.name] = []string{wonka.EnrollerGroup}
					handlerCfg.Pullo = rpc.NewMockPulloClient(mem,
						rpc.Logger(handlerCfg.Logger, zap.NewAtomicLevel()))

					handlers.SetupHandlers(r, handlerCfg)

					privKey := PrivateKeyFromPem(RSAPrivKey)
					pubKey := PublicPemFromKey(k)

					entity := wonka.Entity{
						EntityName:   m.toEnroll,
						ECCPublicKey: eccPublicFromPrivateKey(k),
						PublicKey:    string(pubKey),
						Location:     m.location,
						Requires:     m.requires,
						Ctime:        int(time.Now().Unix()),
					}
					entity = signEntity(t, entity, privKey)

					cfg := wonka.Config{
						EntityName: m.name,
						Agent:      a,
					}

					w, err := wonka.Init(cfg)
					if m.initErr {
						require.Contains(t, err.Error(), m.errMsg, "%d init", idx)
					} else {
						ctx := context.Background()
						err = w.Ping(ctx)
						require.NoError(t, err, "%d, ping %v", idx, err)

						e, err := w.EnrollEntity(ctx, &entity)
						if m.enrollErr {
							require.Contains(t, err.Error(), m.errMsg, "test %d", idx)
						} else {
							require.NoError(t, err, "%d should not errror: %v", idx, err)
							require.Equal(t, m.location, e.Location, "%d, locations don't match", idx)
							require.Equal(t, m.requires, e.Requires, "%d, requires don't match", idx)
						}
					}
				})
			})
		})
	}
}

func TestEnroll(t *testing.T) {
	entityName := "wonkaSample:servicefoo"
	withWonkaInit(t, entityName, func(w wonka.Wonka, e *wonka.Entity) {
		ctx := context.Background()
		_, err := w.Enroll(ctx, "", nil)
		require.NoError(t, err, "should not errror: %v", err)
		// TODO(tjulian): fix this
		//	require.Equal(t, entityName, entity.EntityName, "entity name doesn't match")
		//	require.Equal(t, e.ECCPublicKey, entity.ECCPublicKey, "public key doesn't match")
	})
}

func withWonkaInit(t *testing.T, entityName string, fn func(w wonka.Wonka, e *wonka.Entity)) {
	WithTempDir(func(dir string) {
		k := PrivateKeyFromPem(RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		wonkatestdata.WithWonkaMaster(entityName, func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)
			privKey := PrivateKeyFromPem(RSAPrivKey)
			pubKey := PublicPemFromKey(k)

			entity := wonka.Entity{
				EntityName:   entityName,
				ECCPublicKey: eccPublicFromPrivateKey(k),
				PublicKey:    string(pubKey),
				Ctime:        int(time.Now().Unix()),
			}
			entity = signEntity(t, entity, privKey)
			EnrollEntity(context.Background(), t, handlerCfg.DB, entityName, privKey)

			cfg := wonka.Config{
				EntityName:     entityName,
				PrivateKeyPath: privKeyPath,
			}

			w, err := wonka.Init(cfg)
			require.NoError(t, err, "wonka init %v", err)
			err = w.Ping(context.Background())
			require.NoError(t, err, "ping %v", err)

			fn(w, &entity)
		})
	})
}

func TestLookup(t *testing.T) {
	entityName := "wonkaSample:servicefoo"
	withWonkaInit(t, entityName, func(w wonka.Wonka, e *wonka.Entity) {
		ctx := context.Background()
		_, err := w.Enroll(ctx, "", nil)
		require.NoError(t, err, "should not errror: %v", err)

		entity, err := w.Lookup(ctx, entityName)
		require.NoError(t, err, "should not errror: %v", err)
		require.Equal(t, entityName, entity.EntityName, "entity name doesn't match")
		require.Equal(t, e.ECCPublicKey, entity.ECCPublicKey, "public key doesn't match")
	})
}

func TestSettingMultipleMasterKeys(t *testing.T) {
	compressedKeys := make([]string, 0, 5)
	keys := make([]*ecdsa.PublicKey, 0, 5)

	for i := 0; i < 5; i++ {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, "generating key: %v", err)

		pubKey := wonka.KeyToCompressed(k.PublicKey.X, k.PublicKey.Y)

		compressedKeys = append(compressedKeys, pubKey)
		keys = append(keys, &k.PublicKey)
	}

	defer testhelper.SetEnvVar("WONKA_MASTER_ECC_PUB", strings.Join(compressedKeys, ","))()
	err := wonka.InitWonkaMasterECC()
	require.NoError(t, err, "init error: %v", err)

	require.Equal(t, len(keys), len(wonka.WonkaMasterPublicKeys))
	for idx, k := range wonka.WonkaMasterPublicKeys {
		k1, err := x509.MarshalPKIXPublicKey(k)
		require.NoError(t, err, "marshalling error: %v", err)
		k2, err := x509.MarshalPKIXPublicKey(keys[idx])
		require.NoError(t, err, "marshalling error: %v", err)

		require.True(t, bytes.Equal(k1, k2), "keys should be equal")
	}
}

func TestNewEntityCrypter(t *testing.T) {
	wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
		WithTempDir(func(dir string) {
			handlers.SetupHandlers(r, handlerCfg)
			k := PrivateKeyFromPem(RSAPrivKey)
			privKeyPath := path.Join(dir, "wonka_private.pem")
			e := WritePrivateKey(k, privKeyPath)
			require.NoError(t, e, "writing privkey")

			cfg := wonka.Config{
				EntityName:     "test",
				PrivateKeyPath: privKeyPath,
			}
			EnrollEntity(context.Background(), t, handlerCfg.DB, cfg.EntityName, k)
			w, err := wonka.Init(cfg)
			require.NoError(t, err)
			require.NotNil(t, w)

			c, ok := w.(wonka.Crypter)
			require.True(t, ok)
			require.NotNil(t, c.NewEntityCrypter())
		})
	})
}

func ExampleCrypter() {
	cfg := wonka.Config{
		EntityName: "test",
	}
	w, _ := wonka.Init(cfg)

	// Cast the returned wonka interface to wonka.Crypter to access
	// the additional methods exposed by the Crypter interface.
	_ = w.(wonka.Crypter).NewEntityCrypter()
}

func eccPublicFromPrivateKey(k *rsa.PrivateKey) string {
	h := crypto.SHA256.New()
	h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
	pointKey := h.Sum(nil)

	return wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(pointKey))
}

func signEntity(t *testing.T, e wonka.Entity, privKey *rsa.PrivateKey) wonka.Entity {
	toSign := fmt.Sprintf("%s<%d>%s", e.EntityName, e.Ctime, e.PublicKey)
	h := crypto.SHA256.New()
	h.Write([]byte(toSign))
	sig, err := privKey.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
	require.NoError(t, err, "signing shouldn't error: %v", err)
	e.EntitySignature = base64.StdEncoding.EncodeToString(sig)
	e.SigType = wonka.SHA256

	return e
}
