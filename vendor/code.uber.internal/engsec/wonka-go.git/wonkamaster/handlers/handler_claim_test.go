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
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/testhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestPersonnelClaimRequest(t *testing.T) {
	var testVars = []struct {
		desc           string // Test description.
		explicitClaims string // Request these claims. Comma seperated list.
		errMsg         string
	}{
		{
			desc:           "upper case AD prefix",
			explicitClaims: "AD:x-men",
		},
		{
			desc:           "lower case ad prefix",
			explicitClaims: "ad:x-men",
		},
		{
			desc:           "not member of group",
			explicitClaims: "AD:justice-league",
			errMsg:         "REJECTED_CLAIM_NO_ACCESS",
		},
		{
			desc:           "multiple groups",
			explicitClaims: "AD:x-men,AD:avengers",
		},
	}

	name := "testyMcTestface@uber.com"

	for _, m := range testVars {
		t.Run(m.desc, func(t *testing.T) {
			wonkatestdata.WithUSSHAgent(name, func(agentPath string, caKey ssh.PublicKey) {
				wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
					mem := make(map[string][]string)
					mem[name] = []string{"AD:x-men", "AD:avengers"}

					handlerCfg.Pullo = rpc.NewMockPulloClient(mem,
						rpc.Logger(handlerCfg.Logger, zap.NewAtomicLevel()))
					handlerCfg.Ussh = []ssh.PublicKey{caKey}

					SetupHandlers(r, handlerCfg)

					a, err := net.Dial("unix", agentPath)
					require.NoError(t, err, "ssh-agent dial error")

					w, err := wonka.Init(wonka.Config{EntityName: name, Agent: agent.NewClient(a)})
					require.NoError(t, err, "error initializing wonka")

					_, err = w.ClaimRequest(context.Background(), m.explicitClaims, "some-destination-service")

					if m.errMsg == "" {
						assert.NoError(t, err, "claim request should succeed")
					} else {
						require.Error(t, err, "claim request should fail")
						assert.Contains(t, err.Error(), m.errMsg, "unexpected error")
					}
				})
			})
		})
	}
}

func WithEnrolledEntity(name string, memberships []string, fn func(wonka.Wonka)) {
	defer testhelper.UnsetEnvVar("SSH_AUTH_SOCK")()
	wonkatestdata.WithWonkaMaster(name, func(r common.Router, handlerCfg common.HandlerConfig) {
		SetupHandlers(r, handlerCfg)

		k := wonkatestdata.PrivateKey()
		ecc := crypto.SHA256.New()
		ecc.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
		e := wonka.Entity{
			EntityName:   name,
			PublicKey:    keys.RSAPemBytes(&k.PublicKey),
			ECCPublicKey: wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(ecc.Sum(nil))),
		}

		err := handlerCfg.DB.Create(context.TODO(), &e)
		if err != nil {
			panic("couldn't create entity")
		}

		wonkatestdata.WithTempDir(func(dir string) {
			pubKeyPath := path.Join(dir, "wonka_public")
			privKeyPath := path.Join(dir, "wonka_private")
			err := wonkatestdata.GenerateKeys(pubKeyPath, privKeyPath, k)
			if err != nil {
				panic(err)
			}

			cfg := wonka.Config{
				EntityName:     name,
				PrivateKeyPath: privKeyPath,
			}

			w, err := wonka.Init(cfg)
			if err != nil {
				panic(err)
			}

			fn(w)
		})
	})
}

// TODO(pmoody): this test needs fixing.
func TestClaimMultiClaim(t *testing.T) {
	var claimVars = []struct {
		name string
		with []string
		req  string
		recv []string
		err  bool
	}{
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: wonka.EveryEntity,
			recv: []string{"wonkaSample:test", wonka.EveryEntity}},
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: "EVERYONE,OTHER",
			recv: []string{"wonkaSample:test", wonka.EveryEntity}},
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity},
			req: fmt.Sprintf("%s,%s", wonka.EveryEntity, wonka.NullEntity), err: true},
		{name: "querybuilder", with: []string{wonka.EveryEntity, "KNOXGROUP"},
			req: "EVERYONE,KNOXGROUP", recv: []string{"querybuilder", wonka.EveryEntity, "KNOXGROUP"}},
		{name: "knox", with: []string{"KnoxGroup"}, req: "KNOXGROUP",
			recv: []string{wonka.EveryEntity, "knox", "KNOXGROUP"}},
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: "OTHER", err: true},
		{name: "wonkaSample:test", with: []string{"knoxgroup"}, req: "KNOXGROUP", err: true},
	}

	for idx, m := range claimVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			// create our entity
			WithEnrolledEntity(m.name, m.with, func(w wonka.Wonka) {
				claim, err := w.ClaimRequest(context.Background(), m.req, "foo")
				if m.err {
					require.Error(t, err, "claim request should error and didn't")
				} else {
					require.NoError(t, err, "%d should be able to request a claim: %v", idx, err)
					sort.Strings(claim.Claims)
					sort.Strings(m.recv)
					require.Equal(t, m.recv, claim.Claims, "%d", idx)
				}
			})
		})
	}
}

var claimTestVars = []struct {
	badBody   bool
	badEntity bool
	badKey    bool
	expired   bool
	claim     string

	reply string
	hcMsg string
}{
	{badBody: true, reply: wonka.DecodeError},
	{badEntity: true, reply: "\"ENTITY_UNKNOWN\""},
	{reply: "\"claim_token\""},
	{claim: "ALL", reply: wonka.ClaimRejectedNoAccess},
	{expired: true, reply: wonka.ClaimRequestExpired},
}

func TestClaimHandler(t *testing.T) {
	log := zap.L()

	db := wonkadb.NewMockEntityDB()
	for idx, m := range claimTestVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			wonkatestdata.WithHTTPListener(func(ln net.Listener, r *xhttp.Router) {
				eccPrivateKey := wonkatestdata.ECCKey()     // wonkatestdata
				rsaPrivateKey := wonkatestdata.PrivateKey() // wonkatestdata
				pubKey, err := ssh.NewPublicKey(&rsaPrivateKey.PublicKey)
				require.NoError(t, err, "generating pbkey: %v", err)

				var mem map[string][]string
				handlerCfg := common.HandlerConfig{
					Logger:    log,
					Metrics:   tally.NoopScope,
					DB:        db,
					ECPrivKey: eccPrivateKey,
					Pullo:     rpc.NewMockPulloClient(mem, rpc.Logger(log, zap.NewAtomicLevel())),
					Ussh:      []ssh.PublicKey{pubKey},
				}
				r.AddPatternRoute("/claim", newClaimHandler(handlerCfg))

				name := "admin"
				claimGroups := wonka.EveryEntity
				if m.claim != "" {
					claimGroups = m.claim
				}
				claim := wonkatestdata.NewClaimReq(name, claimGroups)
				ctx := context.TODO()

				k := wonkatestdata.PrivateKey()
				if !m.badEntity {
					entity := wonka.Entity{
						EntityName:   name,
						PublicKey:    keys.RSAPemBytes(&k.PublicKey),
						ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
					}

					err := db.Create(ctx, &entity)
					require.NoError(t, err, "%d createEntity should succeed", idx)
					defer db.Delete(ctx, entity.Name())
				}

				if m.expired {
					claim.Etime = time.Now().Add(-allowedClaimSkew).Add(-time.Minute).Unix()
				}

				toSign, err := json.Marshal(claim)
				require.NoError(t, err)
				sig, err := wonkacrypter.New().Sign(toSign, wonka.ECCFromRSA(k))
				require.NoError(t, err)
				claim.Signature = base64.StdEncoding.EncodeToString(sig)

				c, e := json.Marshal(claim)
				require.NoError(t, e, "json marshal shouldn't fail: %v", e)

				url := fmt.Sprintf("http://%s/claim", ln.Addr().String())
				log.Info("requesting", zap.Stringer("addr", ln.Addr()))

				client := &http.Client{}
				if m.badBody {
					c = []byte("badbody")
				}
				req, _ := http.NewRequest("GET", url, bytes.NewBuffer(c))

				resp, e := client.Do(req)
				require.NoError(t, e, "%d, get: %v", idx, e)

				body, e := ioutil.ReadAll(resp.Body)
				require.NoError(t, e, "%d, reading body: %v", idx, e)
				require.Contains(t, string(body), m.reply, "%d doesn't contain %s", idx, m.reply)
			})
		})
	}
}

var impersonateVars = []struct {
	self                  string
	impersonate           string
	destination           string
	badImpersonator       bool
	badImpersonatedUser   bool
	badGroup              bool
	badKey                bool
	badBody               bool
	expired               bool
	impersonatedUserGroup string
}{
	{self: "usso", impersonate: "e1@uber.com", destination: "wonkaSample:test"},
	{self: "fakeusso", badImpersonator: true, impersonate: "pmoody@uber.com", destination: "wonkaSample:test"},
	{self: "usso", impersonate: "fakeuser@uber.com", badImpersonatedUser: true, destination: "wonkaSample:test"},
	{self: "usso", impersonate: "e2@uber.com", destination: "wonkaSample:test2", impersonatedUserGroup: "fakegroup", badGroup: true},
	{self: "usso", impersonate: "e3@uber.com", destination: "wonkaSample:test", badKey: true},
	{self: "usso", impersonate: "e4@uber.com", destination: "wonkaSample:test", badBody: true},
	{self: "usso", impersonate: "e5@uber.com", destination: "wonkaSample:test", expired: true},
	{self: "usso", impersonate: "e6@uber.com", destination: "wonkaSample:test2", impersonatedUserGroup: wonka.EveryEntity},
}

func TestImpersonateHandler(t *testing.T) {
	log := zap.S()

	impersonatingServices := []string{"usso"}
	for idx, m := range impersonateVars {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			wonkatestdata.WithWonkaMaster(m.self, func(r common.Router, handlerCfg common.HandlerConfig) {
				// make sure our impersonating user is in the engineering group.
				var p map[string][]string

				if m.badImpersonatedUser {
					p = map[string][]string{
						m.impersonate + ".com": {"AD:engineering"},
					}
				} else {
					p = map[string][]string{
						m.impersonate: {"AD:engineering"},
					}
				}

				handlerCfg.Pullo = rpc.NewMockPulloClient(p,
					rpc.Logger(handlerCfg.Logger, zap.NewAtomicLevel()))
				handlerCfg.Imp = impersonatingServices

				SetupHandlers(r, handlerCfg)
				defer testhelper.UnsetEnvVar("SSH_AUTH_SOCK")()

				wonkatestdata.WithTempDir(func(dir string) {
					// Generate keys in temp dir for entity
					pubPath := path.Join(dir, "public.pem")
					privPath := path.Join(dir, "private.pem")
					selfKey := wonkatestdata.PrivateKey()
					if err := wonkatestdata.WritePrivateKey(selfKey, privPath); err != nil {
						log.Fatal("writing privkey", zap.Error(err))
					}
					if err := wonkatestdata.WritePublicKey(&selfKey.PublicKey, pubPath); err != nil {
						log.Fatal("writing pubkey", zap.Error(err))
					}

					privateKeySelf := hashes(privPath)
					ctx := context.TODO()

					log.Info("generated key",
						zap.String("priv", keys.KeyHash(privateKeySelf)),
						zap.String("pub", keys.KeyHash(&privateKeySelf.PublicKey)),
					)

					// Generate keys in temp dir for dest
					pubPathDest := path.Join(dir, "publicdest.pem")
					privPathDest := path.Join(dir, "privatedest.pem")
					DestKey := wonkatestdata.PrivateKey()
					if err := wonkatestdata.WritePrivateKey(DestKey, privPathDest); err != nil {
						log.Fatal("writing privkey", zap.Error(err))
					}
					if err := wonkatestdata.WritePublicKey(&DestKey.PublicKey, pubPathDest); err != nil {
						log.Fatal("writing pubkey", zap.Error(err))
					}

					privateKeyDest := hashes(privPathDest)

					log.Info("generated key",
						zap.String("priv", keys.KeyHash(privateKeySelf)),
						zap.String("pub", keys.KeyHash(&privateKeySelf.PublicKey)),
					)

					self := m.self

					// set destination
					destination := "wonkaSample:test"
					if m.destination != "" {
						destination = m.destination
					}

					// Adding group that impersonatedUser is a member of
					impersonatedUserGroup := "AD:engineering"
					if m.impersonatedUserGroup != "" {
						impersonatedUserGroup = m.impersonatedUserGroup
					}

					// Add destination entity to database
					destEntity := wonka.Entity{
						EntityName:   destination,
						PublicKey:    keys.RSAPemBytes(&privateKeyDest.PublicKey),
						ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(privateKeyDest),
						Requires:     impersonatedUserGroup,
						Ctime:        int(time.Now().Unix()),
						Etime:        int(time.Now().Add(time.Minute).Unix()),
					}
					errDestEntity := handlerCfg.DB.Create(ctx, &destEntity)
					require.NoError(t, errDestEntity, "%d createEntity should succeed", idx)

					// Add "self" as entity in db
					entity := wonka.Entity{
						EntityName:   self,
						PublicKey:    keys.RSAPemBytes(&privateKeySelf.PublicKey),
						ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(privateKeySelf),
						Ctime:        int(time.Now().Unix()),
						Etime:        int(time.Now().Add(time.Minute).Unix()),
					}

					errEntity := handlerCfg.DB.Create(ctx, &entity)
					require.NoError(t, errEntity, "%d createEntity should succeed")

					// Create a new claim request for self to wonkamaster. This is
					// usso's claim request to wonkamaster
					claim := wonkatestdata.NewClaimReq(self, impersonatedUserGroup)
					claim.Destination = destination
					claim.ImpersonatedEntity = m.impersonate

					if m.badKey {
						badKey, e := rsa.GenerateKey(rand.Reader, 1024)
						require.NoError(t, e, "generate bad key: %v", e)
						entity.PublicKey = keys.RSAPemBytes(&badKey.PublicKey)
					}
					defer handlerCfg.DB.Delete(ctx, entity.Name())

					if m.expired {
						claim.Etime = time.Now().Add(-allowedClaimSkew).Add(-time.Minute).Unix()
					}

					toSign, err := json.Marshal(claim)
					require.NoError(t, err, "marhsalling claim for signing: %v", err)

					sig, e := wonkacrypter.New().Sign(toSign, handlerCfg.ECPrivKey)
					require.NoError(t, e, "%d, signing the claim should succeed: %v", idx, e)
					claim.Signature = base64.StdEncoding.EncodeToString(sig)

					url := fmt.Sprintf("http://%s:%s/claim/v2", os.Getenv("WONKA_MASTER_HOST"),
						os.Getenv("WONKA_MASTER_PORT"))
					log.Info("requesting",
						zap.String("host", os.Getenv("WONKA_MASTER_HOST")),
						zap.String("port", os.Getenv("WONKA_MASTER_PORT")),
					)

					client := &xhttp.Client{}

					var resp wonka.ClaimResponse
					err = xhttp.PostJSON(context.Background(), client, url, claim, &resp, nil)
					if m.badImpersonator {
						require.Error(t, err, "should error")
						require.Contains(t, err.Error(), wonka.ClaimInvalidImpersonator, err.Error())
					} else if m.badImpersonatedUser || m.badGroup {
						require.Error(t, err, "should error")
						require.Contains(t, err.Error(), wonka.ClaimRejectedNoAccess, err.Error())
					} else if m.expired {
						require.Error(t, err, "should error")
						require.Contains(t, err.Error(), wonka.ClaimRequestExpired, err.Error())
					} else {
						require.NoError(t, err, "post failure: %v", err)
						require.Equal(t, wonka.ResultOK, resp.Result, "result should be ok")
					}
				})
			})
		})
	}
}

func TestUserAuth(t *testing.T) {

	var userAuthVars = []struct {
		badArgs          bool
		badCert          bool
		noneCert         bool
		badSignature     bool
		invalidSignature bool

		errMsg  string
		reqType claimRequestType
	}{
		{reqType: userClaim},
		{badArgs: true, reqType: invalidClaim,
			errMsg: "user verify: ussh signature check failed"},
		{badCert: true, reqType: invalidClaim,
			errMsg: "parsing ssh key failed"},
		{noneCert: true, reqType: invalidClaim,
			errMsg: "rejecting non-certificate key"},
		{badSignature: true, reqType: invalidClaim,
			errMsg: "signature decoding error"},
		{invalidSignature: true, reqType: invalidClaim,
			errMsg: "ussh signature check failed"},
	}

	for _, m := range userAuthVars {
		t.Run(m.errMsg, func(t *testing.T) {
			name := "admin"
			email := fmt.Sprintf("%s@uber.com", name)
			claim := wonkatestdata.NewClaimReq(email, wonka.EveryEntity)

			signClaimRequest(t, &claim)

			cert, privSigner, authority := generateUSSHCert(name, ssh.UserCert)

			claim.USSHCertificate = string(ssh.MarshalAuthorizedKey(cert))

			addUSSHSignature(t, privSigner, &claim)

			h := claimHandler{
				log:        zap.NewNop(),
				metrics:    tally.NoopScope,
				usshCAKeys: []ssh.PublicKey{authority.PublicKey()},
			}

			// all the ways we can mess this up.
			if m.badArgs {
				claim.USSHSignatureType = ""
			}

			if m.badCert {
				claim.USSHCertificate = "foober"
			}

			if m.noneCert {
				claim.USSHCertificate = string(ssh.MarshalAuthorizedKey(cert.Key))
			}

			if m.badSignature {
				claim.USSHSignature = "foober"
			}

			if m.invalidSignature {
				claim.USSHSignature = base64.StdEncoding.EncodeToString([]byte("foober"))
			}

			_, _, reqType, e := h.usshAuth(&claim)
			if m.errMsg == "" {
				assert.NoError(t, e, "user authentication should succeed")
			} else {
				require.Error(t, e, "user authentication should error")
				assert.Contains(t, e.Error(), m.errMsg, "unexpected error")
			}
			assert.Equal(t, m.reqType, reqType, "unexpected request type")
		})
	}
}

func TestClaimHostClaim(t *testing.T) {
	var hostVars = []struct {
		name        string
		badSigner   bool
		invalidCert bool

		reqType claimRequestType
		errMsg  string
	}{
		{name: "foo01", reqType: hostClaim},
		{name: "foo01", badSigner: true, reqType: invalidClaim,
			errMsg: "no authorities for hostname"},
		{name: "foo01", invalidCert: true, reqType: invalidClaim,
			errMsg: "ussh verify failure: error validating host cert"},
		{name: "localhost", reqType: invalidClaim, errMsg: "invalid entity name"},
	}

	for _, m := range hostVars {
		t.Run(m.errMsg, func(t *testing.T) {
			cert, privSigner, authority := generateUSSHCert(m.name, ssh.HostCert)

			claimGroups := wonka.EveryEntity
			claim := wonkatestdata.NewClaimReq(m.name, claimGroups)

			signClaimRequest(t, &claim)

			h := claimHandler{
				log:                 zap.NewNop(),
				metrics:             tally.NoopScope,
				usshHostKeyCallback: hostCallbackFromPubkey(t, authority.PublicKey(), cert.ValidPrincipals[0]),
			}

			if m.badSigner {
				newName := fmt.Sprintf("%s-bad", cert.ValidPrincipals[0])
				h.usshHostKeyCallback = hostCallbackFromPubkey(t, authority.PublicKey(), newName)
			}

			if m.invalidCert {
				cert.ValidPrincipals[0] = fmt.Sprintf("%s-error", cert.ValidPrincipals[0])
			}

			claim.USSHCertificate = string(ssh.MarshalAuthorizedKey(cert))

			addUSSHSignature(t, privSigner, &claim)

			_, _, reqType, e := h.usshAuth(&claim)
			if m.errMsg == "" {
				assert.NoError(t, e, "host authentication should succeed")
			} else {
				require.Error(t, e, "host authentication should error")
				assert.Contains(t, e.Error(), m.errMsg, "unexpected error")
			}
			assert.Equal(t, m.reqType, reqType, "unexpected request tye")
		})
	}
}

func hostCallbackFromPubkey(t *testing.T, pub ssh.PublicKey, name string) ssh.HostKeyCallback {
	var cb ssh.HostKeyCallback
	wonkatestdata.WithTempDir(func(dir string) {
		knownHosts := path.Join(dir, "known_hosts")
		khContents := fmt.Sprintf("@cert-authority %s %s", name, ssh.MarshalAuthorizedKey(pub))

		var err error
		err = ioutil.WriteFile(knownHosts, []byte(khContents), 0644)
		require.NoError(t, err, "failed to write known hosts file")

		cb, err = knownhosts.New(knownHosts)
		require.NoError(t, err, "error creating host key callback")
	})

	return cb
}

func signClaimRequest(t *testing.T, cr *wonka.ClaimRequest) {
	eccKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating session public key")
	cr.SessionPubKey = wonka.KeyToCompressed(eccKey.PublicKey.X, eccKey.PublicKey.Y)

	toSign, err := json.Marshal(cr)
	require.NoError(t, err, "failed to marshal claim request")

	sig, err := wonkacrypter.New().Sign(toSign, eccKey)
	require.NoError(t, err, "failed to sign claim request")

	cr.Signature = base64.StdEncoding.EncodeToString(sig)
}

func addUSSHSignature(t *testing.T, signer ssh.Signer, cr *wonka.ClaimRequest) {
	toSign, err := json.Marshal(cr)
	require.NoError(t, err, "failed to marshal claim request")

	sig, err := signer.Sign(rand.Reader, toSign)
	require.NoError(t, err, "failed to ussh sign claim request")

	cr.USSHSignature = base64.StdEncoding.EncodeToString(sig.Blob)
	cr.USSHSignatureType = sig.Format
}

func generateUSSHCert(name string, certType uint32) (*ssh.Certificate, ssh.Signer, ssh.Signer) {
	k := wonkatestdata.AuthorityKey()
	authority, err := ssh.NewSignerFromKey(k)
	if err != nil {
		panic(err)
	}

	k = wonkatestdata.PrivateKey()
	private, err := ssh.NewSignerFromKey(k)
	if err != nil {
		panic(err)
	}

	c := &ssh.Certificate{
		CertType:        certType,
		Key:             private.PublicKey(),
		ValidPrincipals: []string{name},
		Serial:          1,
		ValidAfter:      0,
		ValidBefore:     uint64(time.Now().Add(time.Minute).Unix()),
	}
	if err := c.SignCert(rand.Reader, authority); err != nil {
		panic(err)
	}
	return c, private, authority
}
