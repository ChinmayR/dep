package handlers

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

var updateVars = []struct {
	userEnrollment bool
	name           string
	sameKey        bool
	update         bool
	create         bool
}{
	{name: "foober", update: false},
	{name: "wonkaSample:foober", update: true, create: true},
	{name: "foober", userEnrollment: true, update: true, create: true, sameKey: true},
	{name: "foober", userEnrollment: true, update: false, create: true},
}

func TestCanUpdate(t *testing.T) {
	for idx, m := range updateVars {
		// TODO(jkline): Better description for each test
		t.Run(fmt.Sprintf("%d: %+v", idx, m), func(t *testing.T) {
			wonkatestdata.WithWonkaMaster("foober", func(common.Router, common.HandlerConfig) {
				k := wonkatestdata.PrivateKey()
				entity := wonka.Entity{
					EntityName:   m.name,
					PublicKey:    keys.RSAPemBytes(&k.PublicKey),
					ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
				}

				db := wonkadb.NewMockEntityDB()
				dbe := wonka.Entity{EntityName: m.name}
				dbe.PublicKey = keys.RSAPemBytes(&k.PublicKey)
				if !m.sameKey {
					newKey := wonkatestdata.AuthorityKey()
					dbe.PublicKey = keys.RSAPemBytes(&newKey.PublicKey)
				}

				h := enrollHandler{
					log:     zap.L(),
					metrics: tally.NoopScope,
					db:      db,
				}
				_, _, createdErr := h.tryCreate(context.TODO(), entity, m.userEnrollment)
				_, _, updatedErr := h.tryUpdate(context.TODO(), entity, dbe)
				if m.update {
					require.NoError(t, updatedErr, "%d update should succeed", idx)
				} else {
					require.Error(t, updatedErr, "%d update should fail", idx)
				}
				if m.create {
					require.NoError(t, createdErr, "%d create should succeed", idx)
				} else {
					require.Error(t, createdErr, "%d create should fail", idx)
				}
			})
		})
	}
}

var enrollVars = []struct {
	name        string
	sig         bool
	notYetValid bool
	badKey      bool
	badTime     bool
	update      bool
	xwonkaauth  bool

	result string
}{
	{result: wonka.DecodeError},
	{name: "wonkaSample:foober", result: wonka.SignatureVerifyError},
	{name: "foober", sig: true, result: wonka.EnrollNotPermitted},
	{name: "wonkaSample:test", sig: true, result: wonka.ResultOK},
	{name: wonka.NullEntity, sig: true, result: "invalid attempt to register invalid entity"},
	{name: wonka.EveryEntity, sig: true, result: "invalid attempt to register invalid entity"},
	{name: "bad|name", sig: true, result: wonka.EnrollInvalidEntity},
	{name: "AD:wonka-admins", sig: true, result: wonka.EnrollInvalidEntity},
	{name: "user@uber.com", sig: true, result: wonka.EnrollInvalidEntity},
	{name: "wonkaSample:test", sig: true, badKey: true, result: wonka.EnrollInvalidPublicKey},
	{name: "wonkaSample:test", sig: true, badTime: true, result: wonka.ErrTimeWindow},
	{name: "wonkaSample:test", sig: true, result: wonka.ResultOK},
	{name: "wonkaSample:test", sig: true, notYetValid: true, result: wonka.ErrTimeWindow},
}

func TestEnrollHandler(t *testing.T) {
	for idx, m := range enrollVars {
		t.Run(fmt.Sprintf("%d: %+v", idx, m), func(t *testing.T) {
			wonkatestdata.WithWonkaMaster(m.name, func(r common.Router, handlerCfg common.HandlerConfig) {
				SetupHandlers(r, handlerCfg)

				url := fmt.Sprintf("http://%s:%s/enroll", os.Getenv("WONKA_MASTER_HOST"),
					os.Getenv("WONKA_MASTER_PORT"))
				client := &http.Client{Timeout: 2 * time.Second}

				k := wonkatestdata.PrivateKey()
				h := crypto.SHA256.New()
				h.Write([]byte(x509.MarshalPKCS1PrivateKey(k)))
				point := h.Sum(nil)

				eccK := new(ecdsa.PrivateKey)
				eccK.PublicKey.Curve = elliptic.P256()
				eccK.D = new(big.Int).SetBytes(point)

				entity := wonka.Entity{
					EntityName:   m.name,
					PublicKey:    keys.RSAPemBytes(&k.PublicKey),
					ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
					Ctime:        int(time.Now().Unix()),
					Etime:        int(time.Now().Add(2 * time.Minute).Unix()),
					SigType:      "SHA256",
					KeyBits:      0,
				}

				if m.update {
					err := handlerCfg.DB.Create(context.TODO(), &entity)
					require.NoError(t, err, "%d create should succeed", idx)
				}

				if m.notYetValid {
					newCtime := time.Unix(int64(entity.Etime), 0).Add(-time.Millisecond)
					entity.Ctime = int(newCtime.Unix())
				}

				if m.badTime {
					entity.Ctime = int(time.Unix(int64(entity.Etime+60), 0).Unix())
				}

				if m.sig {
					sig, err := keys.SignData(k, "SHA256", fmt.Sprintf("%s<%d>%s",
						entity.EntityName, entity.Ctime, entity.PublicKey))
					require.NoError(t, err, "signing: %v", err)
					entity.EntitySignature = string(sig)
					entity.SigType = "SHA256"
				}

				if m.badKey {
					entity.PublicKey = "bad-rsa-key"
					entity.ECCPublicKey = "bad-ecc-key"
				}

				enrollReq := wonka.EnrollRequest{Entity: &entity}
				b, err := json.Marshal(enrollReq)
				require.NoError(t, err, "%d json marshal error: %v", idx, err)

				if m.name == "" {
					b = nil
				}

				req, _ := http.NewRequest("GET", url, bytes.NewBuffer(b))
				resp, e := client.Do(req)
				require.NoError(t, e, "get")

				body, e := ioutil.ReadAll(resp.Body)
				assert.NoError(t, e, "%d, ReadAll error: %v", idx, e)

				var reply wonka.EnrollResponse
				err = json.Unmarshal(body, &reply)
				require.NoError(t, err, "%d, json unmarshal error: %v", idx, err)
				require.Equal(t, m.result, reply.Result, "test %d", idx)

				if m.result == wonka.ResultOK {
					// verify that an entity got stored
					_, err = handlerCfg.DB.Get(context.TODO(), entity.EntityName)
					require.NoError(t, err, "cassandra entity was not created")
				} else {
					// verify that an entity was not stored
					_, err = handlerCfg.DB.Get(context.TODO(), entity.EntityName)
					require.EqualError(t, err, wonkadb.ErrNotFound.Error(), "cassandra entity was created")
				}
			})
		})
	}
}
