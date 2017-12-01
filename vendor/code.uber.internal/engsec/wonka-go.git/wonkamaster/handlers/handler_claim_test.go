package handlers

import (
	"bytes"
	"context"
	"crypto"
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

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/claims"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func WithEnrolledEntity(name string, memberships []string, fn func(wonka.Wonka)) {
	os.Unsetenv("SSH_AUTH_SOCK")
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
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: wonka.EveryEntity, recv: []string{wonka.EveryEntity}},
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: "EVERYONE,OTHER", recv: []string{wonka.EveryEntity}},
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: fmt.Sprintf("%s,%s", wonka.EveryEntity, wonka.NullEntity),
			err: true},
		{name: "querybuilder", with: []string{wonka.EveryEntity, "KNOXGROUP"}, req: "EVERYONE,KNOXGROUP", recv: []string{wonka.EveryEntity, "KNOXGROUP"}},
		{name: "knox", with: []string{"KnoxGroup"}, req: "KNOXGROUP", recv: []string{"KNOXGROUP"}},
		{name: "wonkaSample:test", with: []string{wonka.EveryEntity}, req: "OTHER", err: true},
		{name: "wonkaSample:test", with: []string{"knoxgroup"}, req: "KNOXGROUP", err: true},
	}

	for idx, m := range claimVars {
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
	{badKey: true, reply: "claim signature check failed: crypto/rsa: verification error"},
	{expired: true, reply: wonka.ClaimRequestExpired},
}

func TestClaimHandler(t *testing.T) {
	log := zap.S()

	db := wonkadb.NewMockEntityDB()
	for idx, m := range claimTestVars {
		wonkatestdata.WithHTTPListener(func(ln net.Listener, r *xhttp.Router) {
			eccPrivateKey := wonkatestdata.ECCKey()     // wonkatestdata
			rsaPrivateKey := wonkatestdata.PrivateKey() // wonkatestdata
			pubKey, err := ssh.NewPublicKey(&rsaPrivateKey.PublicKey)
			require.NoError(t, err, "generating pbkey: %v", err)

			var mem map[string][]string
			handlerCfg := common.HandlerConfig{
				Logger:    zap.L(),
				Metrics:   tally.NoopScope,
				DB:        db,
				ECPrivKey: eccPrivateKey,
				Pullo:     rpc.NewMockPulloClient(mem),
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
					EntityName: name,
					PublicKey:  keys.RSAPemBytes(&k.PublicKey),
				}

				if m.badKey {
					badKey, e := rsa.GenerateKey(rand.Reader, 1024)
					require.NoError(t, e, "generate bad key: %v", e)
					entity.PublicKey = keys.RSAPemBytes(&badKey.PublicKey)
				}

				err := db.Create(ctx, &entity)
				require.NoError(t, err, "%d createEntity should succeed", idx)
				defer db.Delete(ctx, entity.Name())
			}

			if m.expired {
				claim.Etime = time.Now().Add(-allowedClaimSkew).Add(-time.Minute).Unix()
			}

			e := claims.SignClaimRequest(&claim, k)
			require.NoError(t, e, "%d, signing the claim should succeed: %v", idx, e)

			c, e := json.Marshal(claim)
			require.NoError(t, e, "json marshal shouldn't fail: %v", e)

			url := fmt.Sprintf("http://%s/claim", ln.Addr().String())
			log.Infof("requesting: %s", ln.Addr().String())

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

			handlerCfg.Pullo = rpc.NewMockPulloClient(p)
			handlerCfg.Imp = impersonatingServices

			SetupHandlers(r, handlerCfg)
			oldSock := os.Getenv("SSH_AUTH_SOCK")
			os.Unsetenv("SSH_AUTH_SOCK")

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

				log.Infof("generated priv %s, pub %s",
					keys.KeyHash(privateKeySelf), keys.KeyHash(&privateKeySelf.PublicKey))

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

				log.Infof("generated priv %s, pub %s",
					keys.KeyHash(privateKeyDest), keys.KeyHash(&privateKeyDest.PublicKey))

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
				log.Infof("requesting: %s:%s", os.Getenv("WONKA_MASTER_HOST"),
					os.Getenv("WONKA_MASTER_PORT"))

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

				os.Setenv("SSH_AUTH_SOCK", oldSock)
			})
		})
	}
}

var userAuthVars = []struct {
	badArgs          bool
	badCert          bool
	noneCert         bool
	badSignature     bool
	invalidSignature bool

	shouldErr bool
	errMsg    string
	reqType   claimRequestType
}{
	{shouldErr: false, reqType: userClaim},
	{badArgs: true, shouldErr: true, reqType: invalidClaim,
		errMsg: "user verify: ussh signature check failed"},
	{badCert: true, shouldErr: true, reqType: invalidClaim,
		errMsg: "parsing ssh key failed"},
	{noneCert: true, shouldErr: true, reqType: invalidClaim,
		errMsg: "rejecting non-certificate key"},
	{badSignature: true, shouldErr: true, reqType: invalidClaim,
		errMsg: "signature decoding error"},
	{invalidSignature: true, shouldErr: true, reqType: invalidClaim,
		errMsg: "ussh signature check failed"},
}

func TestUserAuth(t *testing.T) {
	for idx, m := range userAuthVars {
		k := wonkatestdata.PrivateKey()

		name := "admin"
		email := fmt.Sprintf("%s@uber.com", name)
		claimGroups := wonka.EveryEntity
		claim := wonkatestdata.NewClaimReq(email, claimGroups)

		e := claims.SignClaimRequest(&claim, k)
		require.NoError(t, e, "signing the claim should succeed: %v", e)

		cert, privSigner, authority := generateUSSHCert(name, ssh.UserCert)
		usshCAKeys := []ssh.PublicKey{authority.PublicKey()}

		claim.USSHCertificate = string(ssh.MarshalAuthorizedKey(cert))
		usshSig := addUSSHSignature(privSigner, claim)
		claim.USSHSignature = base64.StdEncoding.EncodeToString(usshSig.Blob)
		claim.USSHSignatureType = usshSig.Format

		h := claimHandler{
			log:        zap.L(),
			metrics:    tally.NoopScope,
			usshCAKeys: usshCAKeys,
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

		_, _, reqType, e := h.usshAuth(1, &claim)
		require.True(t, (e != nil) == m.shouldErr, "test number %d, %v", idx, e)
		require.Equal(t, m.reqType, reqType, "test number %d", idx)
		if m.errMsg != "" {
			require.Error(t, e, "%d should error", idx)
			require.Contains(t, e.Error(), m.errMsg, "test number %d", idx)
		}
	}
}

func TestClaimHostClaim(t *testing.T) {
	var hostVars = []struct {
		name        string
		badSigner   bool
		invalidCert bool
		shouldErr   bool

		reqType claimRequestType
		errMsg  string
	}{
		{name: "foo01", reqType: hostClaim},
		{name: "foo01", badSigner: true, shouldErr: true, reqType: invalidClaim,
			errMsg: "no authorities for hostname"},
		{name: "foo01", invalidCert: true, shouldErr: true, reqType: invalidClaim,
			errMsg: "ussh verify failure: error validating host cert"},
		{name: "localhost", shouldErr: true, reqType: invalidClaim, errMsg: "invalid entity name"},
	}

	for idx, m := range hostVars {
		k := wonkatestdata.PrivateKey()

		cert, privSigner, authority := generateUSSHCert(m.name, ssh.HostCert)

		claimGroups := wonka.EveryEntity
		claim := wonkatestdata.NewClaimReq(m.name, claimGroups)

		e := claims.SignClaimRequest(&claim, k)
		require.NoError(t, e, "%d, signing the claim should succeed: %v", idx, e)

		h := claimHandler{
			log:                 zap.L(),
			metrics:             tally.NoopScope,
			usshHostKeyCallback: hostCallbackFromPubkey(authority.PublicKey(), cert.ValidPrincipals[0]),
		}

		if m.badSigner {
			newName := fmt.Sprintf("%s-bad", cert.ValidPrincipals[0])
			h.usshHostKeyCallback = hostCallbackFromPubkey(authority.PublicKey(), newName)
		}

		if m.invalidCert {
			cert.ValidPrincipals[0] = fmt.Sprintf("%s-error", cert.ValidPrincipals[0])
		}

		claim.USSHCertificate = string(ssh.MarshalAuthorizedKey(cert))
		usshSig := addUSSHSignature(privSigner, claim)
		claim.USSHSignature = base64.StdEncoding.EncodeToString(usshSig.Blob)
		claim.USSHSignatureType = usshSig.Format

		_, _, reqType, e := h.usshAuth(1, &claim)
		require.True(t, (e != nil) == m.shouldErr, "test number %d, %v", idx, e)
		require.Equal(t, m.reqType, reqType, "test number %d", idx)
		if m.errMsg != "" {
			require.Error(t, e, "%d should error", idx)
			require.Contains(t, e.Error(), m.errMsg, "test number %d", idx)
		}
	}
}

func hostCallbackFromPubkey(pub ssh.PublicKey, name string) ssh.HostKeyCallback {
	var cb ssh.HostKeyCallback
	wonkatestdata.WithTempDir(func(dir string) {
		knownHosts := path.Join(dir, "known_hosts")
		khContents := fmt.Sprintf("@cert-authority %s %s", name, ssh.MarshalAuthorizedKey(pub))

		if err := ioutil.WriteFile(knownHosts, []byte(khContents), 0644); err != nil {
			panic(err)
		}

		var err error
		cb, err = knownhosts.New(knownHosts)
		if err != nil {
			panic(err)
		}
	})

	return cb
}

func addUSSHSignature(signer ssh.Signer, c wonka.ClaimRequest) *ssh.Signature {
	toSign, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}

	sig, err := signer.Sign(rand.Reader, toSign)
	if err != nil {
		panic(err)
	}

	return sig
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
