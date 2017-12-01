package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"
	. "code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

var destroyVars = []struct {
	name   string
	claim  string
	badSig bool

	reply string
	hcMsg string
}{
	{name: "admin", claim: wonka.EveryEntity, reply: "\"OK\""},
	{claim: wonka.EveryEntity, reply: "ENTITY_UNKNOWN"},
	{name: "admin", claim: wonka.EveryEntity, badSig: true}, // "DESTROY Signature Check Failed"
}

func TestDestroyHandler(t *testing.T) {
	for idx, m := range destroyVars {
		WithHTTPListener(func(ln net.Listener, r *xhttp.Router) {
			db := wonkadb.NewMockEntityDB()
			ctx := context.TODO()

			handlerCfg := common.HandlerConfig{
				Logger:  zap.L(),
				Metrics: tally.NoopScope,
				DB:      db,
			}
			r.AddPatternRoute("/destroy", newDestroyHandler(handlerCfg))

			k := PrivateKey()
			entity := wonka.Entity{
				EntityName: m.name,
				PublicKey:  keys.RSAPemBytes(&k.PublicKey),
			}
			err := db.Create(ctx, &entity)
			require.NoError(t, err, "%d, createEntity shouldn't fail for %s", idx, m.name)
			defer db.Delete(ctx, entity.Name())

			toSign := fmt.Sprintf("%s<%d>DESTROY_ENTITY", m.name,
				entity.CreateTime.Unix())
			sig, e := keys.SignData(k, "SHA256", toSign)
			require.NoError(t, e, "%d signing shouldn't fail: %v", idx, e)

			url := fmt.Sprintf("http://%s/destroy?id=%s", ln.Addr().String(), m.name)
			client := &http.Client{
				Timeout: 2 * time.Second,
			}

			if m.badSig {
				sig = []byte("foober")
			}

			req, _ := http.NewRequest("GET", url, bytes.NewBuffer(sig))
			resp, e := client.Do(req)
			require.NoError(t, e, "get")

			b, e := ioutil.ReadAll(resp.Body)
			require.NoError(t, e, "reading reply")
			require.Contains(t, string(b), m.reply, "%d", idx)
		})
	}
}
