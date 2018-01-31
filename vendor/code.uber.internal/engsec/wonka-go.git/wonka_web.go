package wonka

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"code.uber.internal/engsec/wonka-go.git/internal/envfx"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"

	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
)

const (
	bePort = 6746

	adminEndpoint   endpoint = "/admin"
	claimEndpoint   endpoint = "/claim/v2"
	csrEndpoint     endpoint = "/csr"
	enrollEndpoint  endpoint = "/enroll"
	healthEndpoint  endpoint = "/health/json"
	hoseEndpoint    endpoint = "/thehose"
	lookupEndpoint  endpoint = "/lookup"
	resolveEndpoint endpoint = "/resolve"
)

var _wmURLS = []string{
	"http://127.0.0.1:16746", // this covers haproxy and cerberus
}

type endpoint string

var callOpts = &xhttp.CallOptions{CloseRequest: true}

type httpRequester struct {
	baseURL atomic.Value
	client  *xhttp.Client
	log     *zap.Logger
}

func newHTTPRequester(tracer opentracing.Tracer, log *zap.Logger) *httpRequester {
	xtracer := xhttp.Tracer{Tracer: tracer}
	clientFilter := xhttp.ClientFilterFunc(xtracer.TracedClient)
	httpClient := &xhttp.Client{
		// Client timeout is max upper bound. Set a lower timeout using ctx.
		Client: http.Client{Timeout: 10 * time.Second},
		// Explicitly set filter to avoi the default client filter with the
		// default global tracer.
		Filter: clientFilter,
	}

	h := httpRequester{
		client: httpClient,
		log:    log,
	}
	h.baseURL.Store("")
	return &h
}

func (h *httpRequester) SetURL(ctx context.Context, requestedURL string) error {
	// If the URL is already set, bail early
	if h.URL() != "" {
		return nil
	}

	// If Wonkamaster URL is specified in the config
	if requestedURL != "" {
		h.writeURL(requestedURL)
		return nil
	}

	// if WONKA_MASTER_URL is specific as an environment variable, use that.
	if wmURL := os.Getenv("WONKA_MASTER_URL"); wmURL != "" {
		h.writeURL(wmURL)
		return nil
	}

	// If Wonkamaster host & port are specified in environment vars
	// this should be deprecated since it implies http
	host := os.Getenv("WONKA_MASTER_HOST")
	port := os.Getenv("WONKA_MASTER_PORT")
	if host != "" && port != "" {
		h.writeURL(fmt.Sprintf("http://%s:%s", host, port))
		return nil
	}

	// make sure the wonkaURL is nil.
	h.writeURL("")

	// Discover the wonkamaster host & port
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	urls := _wmURLS
	// TODO(pmoody): use envfx when T1263419 is closed
	if zone := envfx.New().Environment.Zone; zone != "" {
		url := fmt.Sprintf("http://wonkamaster-%s.uber.internal:%d", zone, bePort)
		urls = append(urls, url)
	}

	if len(urls) == 0 {
		return errors.New("no wonkamaster urls to test")
	}

	urlLen := len(urls)
	urlChan := make(chan string, urlLen)
	prober := httpProber{
		client: h.client,
		log:    h.log,
	}

	for _, url := range urls {
		go prober.Do(ctx, urlChan, url)
	}

	for u := range urlChan {
		if u != "" {
			h.writeURL(u)
			h.log.Debug("chose url", zap.String("url", u))
			return nil
		}
		urlLen--
		if urlLen == 0 {
			return errors.New("unable to contact wonkamaster on any url. http://t.uber.com/wm-na")
		}
	}

	return errors.New("unreachable code reached")
}

func (h *httpRequester) URL() string {
	return h.baseURL.Load().(string)
}

func (h *httpRequester) writeURL(url string) {
	h.baseURL.Store(url)
}

func (h *httpRequester) Do(ctx context.Context, endpoint endpoint, in, out interface{}) error {
	url := fmt.Sprintf("%s%s", h.URL(), endpoint)
	h.log.Debug("web request", zap.String("url", url))
	// May return nil, or a ResponseError with HTTP response code and body, or other error.
	return xhttp.PostJSON(ctx, h.client, url, in, out, callOpts)
}

type httpProber struct {
	client *xhttp.Client
	log    *zap.Logger
}

// Do tests a given health endpoint for connectivity. If the health endpoint is reachable
// via the provided baseURL, that baseURL is written to the results channel.  Otherwise, an
// empty string is written to results.
func (p *httpProber) Do(ctx context.Context, results chan string, baseURL string) {
	in := struct{}{}
	out := GenericResponse{}

	u := ""
	defer func() {
		results <- u
	}()

	url := fmt.Sprintf("%s%s", baseURL, healthEndpoint)
	p.log.Debug("http probe", zap.String("url", url))
	if err := xhttp.PostJSON(ctx, p.client, url, in, &out, callOpts); err == nil {
		u = baseURL
	}
}
