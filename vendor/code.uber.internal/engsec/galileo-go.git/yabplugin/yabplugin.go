package yabplugin

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"

	"code.uber.internal/engsec/galileo-go.git"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"

	opentracing "github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/yarpc/yab/plugin"
	"github.com/yarpc/yab/transport"
	"go.uber.org/zap"
)

var (
	// used for checking connection to wonkamaster
	wonkamasterTestURL = "http://127.0.0.1:16746/health"

	disableGalileoHelp = "If this error persists, use `--disable-galileo` to run yab with this plugin disabled."
)

type galileoRequestInterceptor struct {
	logger *zap.Logger
	opts   *GalileoOpts
}

// GalileoOpts is a list of command-line flags that are injected into yab
// in order to customize the behavior of the galileo middleware.
type GalileoOpts struct {
	Disabled      bool   `long:"disable-galileo" description:"Disables use of galileo, requests will not be authenticated"`
	RequestClaims string `long:"claims" description:"Request specific claims for the request"`
}

// AddFlags injects GalileoOpts into yab's flag-plugin process, and returns
// a pointer to the injected object.
func AddFlags() *GalileoOpts {
	opts := new(GalileoOpts)
	plugin.AddFlags("Galileo Options", "", opts)
	return opts
}

// NewRequestInterceptor returns a outbound middleware that satisfies yab's
// RequestInterceptor interface.
func NewRequestInterceptor(logger *zap.Logger, opts *GalileoOpts) transport.RequestInterceptor {
	if opts == nil {
		opts = new(GalileoOpts)
	}
	return &galileoRequestInterceptor{
		logger: logger,
		opts:   opts,
	}
}

// Apply adds authentication metadata to the request before sending it out.
func (ri *galileoRequestInterceptor) Apply(ctx context.Context, req *transport.Request) (*transport.Request, error) {
	// make sure we're not disabled
	if ri.opts.Disabled {
		return req, nil
	}

	// test that wonkamaster is reachable
	if !ri.isWonkamasterReachable() {
		return nil, yabErrorf("Error connecting to wonkamaster. Is cerberus running?")
	}

	// we get the user's LDAP username from the environment, which is required to get a valid claim
	user, ok := getUberOwner()
	if !ok {
		return nil, yabErrorf("Error getting your username, please set UBER_OWNER=<username>@uber.com in your environment")
	}

	// test that the user has a valid ussh certificate
	if err := isUsshCertValid(); err != nil {
		return nil, yabErrorf("Error getting user ussh cert (do you have one?): %v", err)
	}

	// we're not part of the standard request-response lifecycle, so create a default jaeger tracer
	tracer, closer := jaeger.NewTracer(user, jaeger.NewConstSampler(true), jaeger.NewNullReporter())
	defer ri.runAndLogErr(closer.Close)

	// galileo initialization
	g, err := galileo.Create(galileo.Configuration{
		ServiceName: user,
		Tracer:      tracer,
		Logger:      ri.logger,
	})
	if err != nil {
		return nil, yabErrorf("Error creating galileo: %v", err)
	}

	// get requested claims
	claims := galileo.EveryEntity
	if ri.opts.RequestClaims != "" {
		claims = ri.opts.RequestClaims
	}

	// use galileo to get authentication baggage
	ctx, err = g.AuthenticateOut(ctx, req.TargetService, claims)
	if err != nil {
		return nil, yabErrorf("Error authenticating: %v", err)
	}
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return nil, yabErrorf("Error galileo auth span is empty")
	}

	// set baggage on the request object
	if req.Baggage == nil {
		req.Baggage = make(map[string]string)
	}
	req.Baggage[contexthelper.ServiceAuthBaggageAttr] = span.BaggageItem(contexthelper.ServiceAuthBaggageAttr)
	return req, nil
}

// returns an error wrapped with information about how to disable this plugin
func yabErrorf(format string, a ...interface{}) error {
	errString := fmt.Sprintf(format, a...)
	return fmt.Errorf("%s. %v", errString, disableGalileoHelp)
}

// tests if wonkamaster can be reached
func (ri *galileoRequestInterceptor) isWonkamasterReachable() bool {
	res, err := http.Get(wonkamasterTestURL)
	if err != nil {
		return false
	}
	defer ri.runAndLogErr(res.Body.Close)
	return res.StatusCode == http.StatusOK
}

func (ri *galileoRequestInterceptor) runAndLogErr(fn func() error) {
	if err := fn(); err != nil {
		ri.logger.Error("Error during shutdown", zap.Error(err))
	}
}

// tests if UBER_OWNER is set to something that resembles an email address (e.g. 'foo@bar.baz')
func getUberOwner() (string, bool) {
	uberOwner := os.Getenv("UBER_OWNER")
	matched, err := regexp.MatchString(".{1,}@.{1,}\\..{1,}", uberOwner)
	if err != nil {
		return "", false
	}
	return uberOwner, matched
}
