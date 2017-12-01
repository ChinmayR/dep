package handlers

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

func TestHealthHandler(t *testing.T) {
	wonkatestdata.WithHTTPListener(func(ln net.Listener, r *xhttp.Router) {
		handlerCfg := common.HandlerConfig{
			Logger:  zap.L(),
			Metrics: tally.NoopScope,
		}

		r.AddPatternRoute("/health", newHealthHandler(handlerCfg))
		url := fmt.Sprintf("http://%s/health", ln.Addr().String())
		client := &http.Client{}
		req, _ := http.NewRequest("GET", url, nil)

		resp, e := client.Do(req)
		require.NoError(t, e, "get: %v", e)

		body, e := ioutil.ReadAll(resp.Body)
		require.NoError(t, e, "%d, reading body: %v", e)
		require.Contains(t, string(body), "OK")
	})
}
