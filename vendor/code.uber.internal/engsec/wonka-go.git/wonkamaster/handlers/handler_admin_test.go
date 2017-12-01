package handlers

import (
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/x509"
	"net"
	"os"
	"path"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var adminHandlerVars = []struct {
	mem    string
	entity string
	toDel  string

	badBaggage bool
	errMsg     string
}{
	{entity: "e1@uber.com", toDel: "e2@uber.com", mem: "AD:wonka-admins"},
	{entity: "e1@uber.com", toDel: "e2@uber.com", mem: "AD:engineering", errMsg: wonka.AdminAccessDenied},
}

func TestAdminHandler(t *testing.T) {
	log := zap.S()

	for idx, m := range adminHandlerVars {
		wonkatestdata.WithUSSHAgent(m.entity, func(agentPath string, caKey ssh.PublicKey) {
			wonkatestdata.WithWonkaMaster(m.entity, func(r common.Router, handlerCfg common.HandlerConfig) {
				os.Setenv("SSH_AUTH_SOCK", agentPath)
				aSock, err := net.Dial("unix", agentPath)
				if err != nil {
					panic(err)
				}

				a := agent.NewClient(aSock)
				k, err := a.List()
				if err != nil {
					panic(err)
				}
				if len(k) != 1 {
					log.Fatalf("invalid keys: %d\n", len(k))
				}

				mem := make(map[string][]string, 0)
				mem[m.entity] = []string{m.mem}
				handlerCfg.Pullo = rpc.NewMockPulloClient(mem)
				handlerCfg.Ussh = []ssh.PublicKey{caKey}

				// put it in the ether
				oldCA := os.Getenv("WONKA_USSH_CA")
				os.Setenv("WONKA_USSH_CA", string(ssh.MarshalAuthorizedKey(caKey)))
				defer os.Setenv("WONKA_USSH_CA", oldCA)

				// here we should be able to request wonka-admin personnel claims
				_, ok := addEntityToDB(m.entity, handlerCfg.DB)
				require.True(t, ok, "test %d create should succeed", idx)
				wonkaCfg := wonka.Config{
					EntityName: m.entity,
				}

				SetupHandlers(r, handlerCfg)

				w, err := wonka.Init(wonkaCfg)
				require.NoError(t, err, "test %d, wonka init error: %v", idx, err)

				_, ok = addEntityToDB(m.toDel, handlerCfg.DB)
				require.True(t, ok, "test %d create should succeed", idx)

				req := wonka.AdminRequest{
					Action:     wonka.DeleteEntity,
					EntityName: m.entity,
					ActionOn:   m.toDel,
				}

				err = w.Admin(context.Background(), req)
				if m.errMsg == "" {
					require.NoError(t, err, "test %d, Admin error: %v", idx, err)
				} else {
					require.True(t, err != nil, "test %d should error", idx)
					require.Contains(t, err.Error(), m.errMsg, "test %d error not equal", idx)
				}
			})
		})
	}
}

func addEntityToDB(entity string, db wonkadb.EntityDB) (string, bool) {
	log := zap.L()

	ok := false
	privPath := ""
	wonkatestdata.WithTempDir(func(dir string) {
		privKey := wonkatestdata.PrivateKey()
		privPath = path.Join(dir, "private.pem")
		pubPath := path.Join(dir, "public.pem")
		if err := wonkatestdata.WritePrivateKey(privKey, pubPath); err != nil {
			log.Fatal("writing privkey", zap.Error(err))
		}
		if err := wonkatestdata.WritePublicKey(&privKey.PublicKey, pubPath); err != nil {
			log.Fatal("writing pubkey", zap.Error(err))
		}

		ecc := crypto.SHA256.New()
		ecc.Write([]byte(x509.MarshalPKCS1PrivateKey(privKey)))

		e := wonka.Entity{
			EntityName:   entity,
			PublicKey:    keys.RSAPemBytes(&privKey.PublicKey),
			ECCPublicKey: wonka.KeyToCompressed(elliptic.P256().ScalarBaseMult(ecc.Sum(nil))),
			Ctime:        int(time.Now().Unix()),
			Etime:        int(time.Now().Add(time.Minute).Unix()),
		}

		err := db.Create(context.TODO(), &e)
		ok = err == nil
	})
	return privPath, ok
}
