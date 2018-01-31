package galileo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/atomic"
	"code.uber.internal/engsec/galileo-go.git/internal/claimtools"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
)

const (
	// TODO(jkline): Use the same value from wonka-go either by exporting the
	// constant from wonka-go, or exposing some sort of claim.ShouldRenew() helper.
	clockSkew = time.Minute
	// Do not reuse a claim token that expires within claimExpiryBuffer of now.
	// 3 chosen arbitrarilly as a reasonable buffer.
	// claim.Valid already allows one clockSkew.
	claimExpiryBuffer = 3 * clockSkew
)

// Variables to allow injecting mocks for testability.
var (
	_newInboundTelemetryReporter  = telemetry.NewInboundReporter
	_newOutboundTelemetryReporter = telemetry.NewOutboundReporter
)

type skippedEntity struct {
	start time.Time
	until time.Duration
}

// uberGalileo implements the Galileo interface
type uberGalileo struct {
	log     *zap.Logger
	metrics tally.Scope
	tracer  opentracing.Tracer

	serviceName    string
	serviceAliases []string
	w              wonka.Wonka

	allowedEntities []string
	endpointCfg     map[string]EndpointCfg

	skippedEntities map[string]skippedEntity
	skipLock        *sync.RWMutex

	cachedClaims map[string]*wonka.Claim
	claimLock    *sync.Mutex

	// enforcePercentage is the percentage of traffic for which we
	// require auth baggege. set this to 0.0 to require no auth baggage
	// and 1 to require auth baggage for all traffic.
	enforcePercentage *atomic.Float64

	// disabled set to true if we shouldn't make any auth requests or
	// expect inbound requests to have auth tokens. This is set by the
	// galileo configuration option, as opposed to the wonka panic
	// button
	disabled bool

	inboundClaimCache *claimtools.InboundCache
}

func (u *uberGalileo) isDisabled() bool {
	if u.disabled {
		// No metric when disabled by configuration.
		return true
	}

	if wonka.IsGloballyDisabled(u.w) {
		u.metrics.Tagged(map[string]string{
			"disabled": "true",
		}).Counter("wonka_disabled").Inc(1)

		return true
	}

	// No metric when enabled.
	return false
}

func (u *uberGalileo) Name() string {
	if u == nil {
		return ""
	}
	return u.serviceName
}

func (u *uberGalileo) Endpoint(endpoint string) (EndpointCfg, error) {
	if e, ok := u.endpointCfg[endpoint]; ok {
		return e, nil
	}
	return EndpointCfg{}, errors.New("no configuration for endpoint")
}

// AuthenticateOut is called by the client to make an authenticated request to a particular destination.
// It returns an authenticated context with the proper baggage.
func (u *uberGalileo) AuthenticateOut(ctx context.Context, destination string, explicitClaims ...interface{}) (context.Context, error) {
	if destination == "" {
		err := errors.New("no destination service specified")
		u.log.Error("AuthenticateOut caller error", zap.Error(err), jaegerzap.Trace(ctx))
		return ctx, err
	}

	explicitClaim, err := getExplicitClaim(ctx, explicitClaims...)
	if err != nil {
		u.log.Error("AuthenticateOut caller error", zap.Error(err), jaegerzap.Trace(ctx))
		return ctx, err
	}

	ctx = u.doAuthenticateOut(ctx, destination, explicitClaim)
	return ctx, nil // T1314721 always succeed, even without auth baggage.
}

// getExplicitClaim examines the given context and variadic explicit claims and
// returns the claim to request.
func getExplicitClaim(ctx context.Context, explicitClaim ...interface{}) (string, error) {
	if len(explicitClaim) > 1 {
		// multiple claims should be in the form of a comma-separated string.
		return "", errors.New("only one explicit claim is supported")
	}

	if len(explicitClaim) == 1 {
		// the provided explicit claim should take precedence over the claim configured on the context.
		c := explicitClaim[0]
		if explicitClaim, ok := c.(string); ok {
			return explicitClaim, nil
		}
		return "", fmt.Errorf("unexpected argument of type %T passed: %v", c, c)
	}

	c := ctx.Value(_ctxClaimKey)
	if c == nil {
		// It is OK not to have an explicit claim.
		return "", nil
	}
	if explicitClaim, ok := c.(string); ok {
		return explicitClaim, nil
	}
	return "", fmt.Errorf("unexpected value of type %T passed as explicit claim in context: %v", c, c)
}

// doAuthenticateOut attempts to add wonka token to the context and reports the
// results through logs and metrics.
func (u *uberGalileo) doAuthenticateOut(inCtx context.Context, destination string, explicitClaim string) (ctx context.Context) {
	var err error // T1314721 err will be reported but not returned.
	otr := _newOutboundTelemetryReporter(u.log, u.metrics, destination, explicitClaim, u.Name())
	// Wrapper func used because value of ctx and err change during doAuthenticateOut.
	// otr.Report will handle if either ends up nil or if ctx still has no span.
	defer func() { otr.Report(ctx, err) }()

	if u.isDisabled() || u.shouldSkipDest(destination) {
		return inCtx
	}

	// New child context with a span that can be used to attach baggage and
	// decorated with logs. This span will cover the claim resolve to
	// wonkamaster, as well as the authenticated request to the destination
	// service.
	ctx, finishSpan := contexthelper.AddSpan(inCtx, u.tracer)
	defer finishSpan()

	claimToken, err := u.doGetCredential(ctx, destination, explicitClaim)
	if err != nil {
		return ctx
	}

	contexthelper.SetBaggage(opentracing.SpanFromContext(ctx), claimToken)
	otr.SetHasBaggage(true)
	return ctx
}

// AuthenticateIn is called by the client to check if the given context contains
// proper authentication baggage. Auth baggage will be removed from the context.
//
// See Galileo interface docs for information about the optional parameters.
func (u *uberGalileo) AuthenticateIn(ctx context.Context, allowed ...interface{}) error {
	if u.isDisabled() {
		return nil
	}

	cfg := validationConfiguration{
		AllowedDestinations: u.serviceAliases,
	}

	var allowedEntities []string
	for _, e := range allowed {
		if e == nil {
			continue
		}
		switch v := e.(type) {
		case string:
			cfg.AllowedEntities = append(cfg.AllowedEntities, v)
		case []string:
			// Backwards compatibility with old calling convention
			cfg.AllowedEntities = append(cfg.AllowedEntities, v...)
		case CredentialValidationOption:
			v.applyCredentialValidationOption(&cfg)
		default:
			return fmt.Errorf("unexpected argument of type %T passed to AuthenticateIn: %v", e, e)
		}
	}

	if len(cfg.AllowedEntities) == 0 {
		cfg.AllowedEntities = u.allowedEntities
	}

	if err := u.doAuthenticateIn(ctx, cfg); err != nil {
		return &authError{
			Reason:          err,
			AllowedEntities: allowedEntities,
		}
	}

	return nil
}

// doAuthenticateIn checks if the given context contains proper authentication
// baggage, and reports the result.
func (u *uberGalileo) doAuthenticateIn(ctx context.Context, cfg validationConfiguration) (err internal.InboundAuthenticationError) {
	// Ensure the context has a span that can be examined for auth baggage and
	// decorated with logs.
	ctx, finishSpan := contexthelper.EnsureSpan(ctx, u.tracer)
	defer finishSpan()

	// State of itr at the time this function ends will dicate what telemetry we
	// send.
	itr := _newInboundTelemetryReporter(u.log, u.metrics, u.enforcePercentage.Load())
	defer func() {
		enforced := shouldEnforce(u.enforcePercentage.Load())
		derelict := u.IsDerelict(cfg.CallerName)
		itr.Report(ctx, err, enforced, derelict)
		if !enforced || derelict {
			err = nil // Not enforced. doAuthenticateIn should succeed.
		}
	}()

	cacheableClaim, err := u.inboundClaimCache.GetOrCreateFromContext(ctx)
	if err != nil {
		return err
	}
	itr.SetClaim(cacheableClaim.Claim)

	if err := validateClaim(cacheableClaim, cfg); err != nil {
		return err
	}

	if cfg.CallerName != "" && !strings.EqualFold(cfg.CallerName, cacheableClaim.EntityName) {
		// Unauthenticated caller name is different from authenticated Wonka
		// identity.
		u.log.Info("remote entity name mismatch",
			zap.String("remote_entity", cacheableClaim.EntityName),
			zap.String("caller_name", cfg.CallerName),
		)
	}

	return nil
}

// resolveClaimWithCache uses a claim from cache when it is still valid, and
// otherwise fetches a new claim from wonkamaster.
func (u *uberGalileo) resolveClaimWithCache(ctx context.Context, destination, explicitClaim string) (*wonka.Claim, error) {
	u.claimLock.Lock()
	defer u.claimLock.Unlock()

	if claim, ok := u.cachedClaims[destination]; ok {
		err := claimUsable(claim, explicitClaim)
		if err == nil {
			u.metrics.Tagged(map[string]string{"cache": "hit"}).Counter("cache").Inc(1)
			u.log.Debug("cached claim still valid",
				zap.String("destination", destination),
				zap.String("claim", explicitClaim),
			)
			return claim, nil
		}
		u.log.Debug("cached claim is no longer valid",
			zap.NamedError("reason", err),
			zap.String("destination", destination),
			zap.String("claim", explicitClaim),
		)
	}

	u.metrics.Tagged(map[string]string{"cache": "miss"}).Counter("cache").Inc(1)

	claim, err := u.resolveClaim(ctx, destination, explicitClaim)
	if err != nil {
		return nil, fmt.Errorf("error requesting claim: %v", err)
	}

	u.cachedClaims[destination] = claim
	u.log.Debug("adding claim to cache",
		zap.String("destination", destination),
		zap.String("claim", explicitClaim),
	)

	return claim, nil
}

// resolveClaim fetches a new claim from wonkamaster
func (u *uberGalileo) resolveClaim(ctx context.Context, destination, explicitClaim string) (*wonka.Claim, error) {
	if explicitClaim != "" {
		return u.w.ClaimRequest(ctx, explicitClaim, destination)
	}

	if strings.ToLower(destination) == strings.ToLower(u.serviceName) {
		return u.w.ClaimRequest(ctx, destination, destination)
	}

	c, err := u.w.ClaimResolve(ctx, destination)
	if err != nil {
		u.addSkipDest(destination)
	}
	return c, err
}

// shouldSkipDest returns true if we should skip wonka auth for this destination
func (u *uberGalileo) shouldSkipDest(entity string) bool {
	u.skipLock.RLock()
	defer u.skipLock.RUnlock()

	t, ok := u.skippedEntities[entity]
	if !ok {
		return false
	}

	if time.Now().After(t.start.Add(t.until)) {
		return false
	}

	return true
}

// addSkipDest adds a new destination to our map of destinations for which we avoid
// wonka auth.
func (u *uberGalileo) addSkipDest(entity string) {
	u.log.Debug("adding skip", zap.String("destination", entity))

	u.skipLock.Lock()
	defer u.skipLock.Unlock()

	newSkip := skippedEntity{start: time.Now(), until: initialSkipDuration}
	e, ok := u.skippedEntities[entity]
	if ok {
		u.log.Debug("entity exists",
			zap.String("destination", entity),
			zap.Duration("until", e.until),
		)

		// if the existing entry expired in the last minute, we assume
		// this a continuation
		if time.Now().Add(-time.Minute).Before(e.start.Add(e.until)) {
			newUntil := 2 * e.until
			if newUntil > maxSkipDuration {
				newUntil = maxSkipDuration
			}
			newSkip.until = newUntil
		}
	}

	u.skippedEntities[entity] = newSkip
}

// claimUsable returns nil if the given claim token can still be used, i.e.
// won't expire soon, and asserts explicitClaim. Returns an error otherwise.
// We assume only valid claims are placed into the cache.
func claimUsable(claim *wonka.Claim, explicitClaim string) error {
	// We already know the token is not expired. Check if it will expire soon.
	expireTime := time.Unix(claim.ValidBefore, 0)
	now := time.Now()
	if now.Add(claimExpiryBuffer).After(expireTime) {
		return errors.New("claim token will expire soon")
	}

	// We have a claim in our cache that matches the destination.
	// When given an explicit claim requirement, only reuse cached claim
	// when it affirms that explictly requested claim.
	if explicitClaim == "" {
		// No explicit claim request, any token will do.
		return nil
	}
	for _, c := range claim.Claims {
		if strings.EqualFold(explicitClaim, c) {
			return nil
		}
	}

	return fmt.Errorf("claim token does not grant %q", explicitClaim)
}
