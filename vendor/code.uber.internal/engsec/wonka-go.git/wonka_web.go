package wonka

import (
	"context"
	"fmt"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"

	"go.uber.org/zap"
)

const (
	prodURL      = "http://127.0.0.5:16746"
	localhostURL = "http://127.0.0.1:16746"
	externalURL  = "https://wonkabar.uber.com"

	adminEndpoint   endpoint = "/admin"
	claimEndpoint   endpoint = "/claim/v2"
	csrEndpoint     endpoint = "/csr"
	enrollEndpoint  endpoint = "/enroll"
	healthEndpoint  endpoint = "/health/json"
	hoseEndpoint    endpoint = "/thehose"
	lookupEndpoint  endpoint = "/lookup"
	resolveEndpoint endpoint = "/resolve"
)

type endpoint string

var callOpts = &xhttp.CallOptions{CloseRequest: true}

func (w *uberWonka) httpRequest(ctx context.Context, endpoint endpoint, in, out interface{}) error {
	reqURL := fmt.Sprintf("%s%s", w.wonkaURL, endpoint)

	return w.httpRequestWithURL(ctx, reqURL, in, out)
}

func (w *uberWonka) httpRequestWithURL(ctx context.Context, url string, in, out interface{}) error {
	w.log.Debug("web request", zap.String("url", url))

	// May return nil, or a ResponseError with HTTP response code and body, or other error.
	return xhttp.PostJSON(ctx, w.httpClient, url, in, out, callOpts)
}
