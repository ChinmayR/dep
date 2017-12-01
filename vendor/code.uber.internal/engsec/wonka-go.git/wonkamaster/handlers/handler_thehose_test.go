package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
)

// TODO(pmoody): this test should be split up into helpers to make it
// easier to test corner cases.
func TestTheHose(t *testing.T) {
	wonkatestdata.WithWonkaMaster("", func(r common.Router, cfg common.HandlerConfig) {
		d := make(map[string]string)
		// these dates don't actually matter
		d["foo"] = "2017/09/01"
		d["bar"] = "2018/09/01"
		badDateEntity := "improperllyFormatted"
		d[badDateEntity] = "2018-09-01"
		cfg.Derelicts = d

		SetupHandlers(r, cfg)

		// everything above here is setup

		url := fmt.Sprintf("http://%s:%s/thehose", os.Getenv("WONKA_MASTER_HOST"),
			os.Getenv("WONKA_MASTER_PORT"))

		var resp wonka.TheHoseReply
		client := &xhttp.Client{}

		err := xhttp.GetJSON(context.Background(), client, url, &resp, nil)
		require.NoError(t, err, "should not error")

		// ensure the response is properlly signed
		toVerifyResp := resp
		toVerifyResp.Signature = nil
		toVerify, err := json.Marshal(toVerifyResp)
		require.NoError(t, err, "error marshalling json: %v", err)

		ok := wonkacrypter.VerifyAny(toVerify, resp.Signature, wonka.WonkaMasterPublicKeys)
		require.True(t, ok, "verify error")
		require.Equal(t, len(d)-1, len(resp.Derelicts))

		// ensure that only valid expiration dates are included
		badDate := false
		for k := range resp.Derelicts {
			if strings.EqualFold(k, badDateEntity) {
				badDate = true
			}
		}
		require.False(t, badDate, "improperlly formatted date should not have been included")
	})
}
