package galileo

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	opentracing "github.com/opentracing/opentracing-go"
	opentracinglog "github.com/opentracing/opentracing-go/log"

	"github.com/uber-go/tally"
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

type skippedEntity struct {
	start time.Time
	until time.Duration
}

// uberGalileo implements the Galileo interface
type uberGalileo struct {
	log     *zap.Logger
	metrics tally.Scope
	tracer  opentracing.Tracer

	serviceName string
	w           wonka.Wonka

	allowedEntities []string
	endpointCfg     map[string]EndpointCfg

	skippedEntities map[string]skippedEntity
	skipLock        *sync.RWMutex

	cachedClaims map[string]*wonka.Claim
	claimLock    *sync.Mutex

	// enforcePercentage is the percentage of traffic for which we
	// require auth baggege. set this to 0.0 to require no auth baggage
	// and 1 to require auth baggage for all traffic.
	enforcePercentage float32

	// disabled set to 1 if for some reason we should not
	// attempt to do any wonka operations. This is true for both
	// outbound claim requests and inbound claim verifications.
	// TODO(pmoody): switch to go.uber.org/atomic to use atomic.Bool
	disabled int32
}

// doDisableLookup
func (u *uberGalileo) doDisableLookup(record string) bool {
	r, err := net.LookupTXT(record)
	if err != nil {
		// if the txt record doesn't exist, this will err
		return false
	}

	recordValue := strings.Join(r, " ")
	reply, err := base64.StdEncoding.DecodeString(recordValue)
	if err != nil {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "record_base64_decode",
		}).Counter("disabled").Inc(1)

		u.log.Error("error base64 decoding", zap.Error(err))
		return false
	}

	var msg wonka.DisableMessage
	if err := json.Unmarshal(reply, &msg); err != nil {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "json_unmarshal",
		}).Counter("disabled").Inc(1)
		u.log.Error("error unmarshalling disable txt record", zap.Error(err))
		return false
	}

	now := time.Now()
	cTime := time.Unix(msg.Ctime, 0)
	eTime := time.Unix(msg.Etime, 0)
	// check that it was created before now
	if cTime.After(now) {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "not_yet_valid",
		}).Counter("disabled").Inc(1)
		u.log.Error("disable message not yet valid", zap.Time("ctime", cTime))
		return false
	}

	// check that it hasn't expried
	if eTime.Before(now) {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "expired",
		}).Counter("disabled").Inc(1)
		u.log.Error("disable message expired", zap.Time("etime", eTime))
		return false
	}

	// check that the disable message isn't good for more than 24 hours
	if !cTime.Add(maxDisableDuration).After(eTime) {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "disable_too_long",
		}).Counter("disabled").Inc(1)
		u.log.Error("disable message is good for too long", zap.Time("etime", eTime))
		return false
	}

	verify := msg
	verify.Signature = nil
	toVerify, err := json.Marshal(verify)
	if err != nil {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "json_marshal",
		}).Counter("disabled").Inc(1)
		u.log.Error("unable to marshal msg to verify", zap.Error(err))
		return false
	}

	ok := wonkacrypter.New().Verify(toVerify, msg.Signature, wonka.WonkaMasterPublicKey)
	if !ok {
		u.metrics.Tagged(map[string]string{
			"record": recordValue,
			"error":  "invalid_signature",
		}).Counter("disabled").Inc(1)
	}

	return ok
}

func (u *uberGalileo) checkDisableStatus(k *ecdsa.PublicKey) {
	keyBytes, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		u.log.Error("error marshalling key", zap.Error(err))
		// TODO(pmoody): should we die here?
		return
	}

	h := crypto.SHA256.New()
	h.Write(keyBytes)
	record := base64.RawStdEncoding.EncodeToString(h.Sum(nil))

	for {
		ok := u.doDisableLookup(fmt.Sprintf("%s.uberinternal.com", record))
		if ok {
			atomic.StoreInt32(&u.disabled, 1)
		}
		time.Sleep(disableCheckPeriod)
	}
}

func (u *uberGalileo) isDisabled() bool {
	d := atomic.LoadInt32(&u.disabled) == 1

	if d {
		u.metrics.Tagged(map[string]string{
			"disabled": "true",
		}).Counter("disabled").Inc(1)
	}

	return d
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
	return EndpointCfg{}, errors.New("galileo: no configuration for endpoint")
}

// Authenticate is called by the client to make an authenticated request to a particular destination.
// It returns an authenticated context with the proper baggage.
func (u *uberGalileo) AuthenticateOut(ctx context.Context, destination string, explicitClaim ...interface{}) (context.Context, error) {
	if u.isDisabled() {
		u.log.Warn("global disable set",
			zap.String("destination", destination),
			zap.String("explicit_claim", fmt.Sprintf("%v", explicitClaim)),
		)
		return ctx, nil
	}

	if u.shouldSkipDest(destination) {
		return ctx, nil
	}

	// we would normally switch through the various authentication types
	// based on the configured contexts.
	claimReq, _ := ctx.Value(_ctxClaimKey).(string)
	if len(explicitClaim) > 0 {
		// the provided explicit claim should take precedence over the claim configured on the context.
		if len(explicitClaim) > 1 {
			// multiple claims should be in the form of a comma-separated string.
			return ctx, fmt.Errorf("only one explicit claim is supported")
		}

		c, ok := explicitClaim[0].(string)
		if !ok {
			u.log.Error("explicit claim",
				zap.String("destination", destination),
				zap.String("explicit_claim", fmt.Sprintf("%v", explicitClaim)),
			)
			return ctx, errors.New("bad explicit claim request")
		}
		claimReq = c
	}

	if destination == "" {
		u.log.Error("no destination service or destination claim specified")
		return ctx, errors.New("no destination")
	}

	// Add a span to the context that can be used to attach baggage and
	// decorated with logs. This span will cover the claim resolve to
	// wonkamaster, as well as the authenticated request to the destination
	// service.
	ctx, finishSpan := internal.AddSpan(ctx, u.tracer)
	defer finishSpan()

	u.claimLock.Lock()
	defer u.claimLock.Unlock()

	if claim, ok := u.cachedClaims[destination]; ok {
		if err := claimUsable(claim, claimReq); err == nil {
			claimBytes, err := wonka.MarshalClaim(claim)
			if err != nil {
				return ctx, fmt.Errorf("marshalling claim: %v", err)
			}

			u.log.Debug("cached claim still valid")

			err = internal.SetBaggage(ctx, u.Name(), destination, string(claimBytes))
			return ctx, err
		} else {
			u.log.Info("deleting invalid claim", zap.String("destination", destination))
			delete(u.cachedClaims, destination)
		}
	}

	claim, err := u.resolveClaim(ctx, destination, claimReq)
	if err != nil {
		return ctx, fmt.Errorf("error requesting claim: %v", err)
	}
	u.cachedClaims[destination] = claim

	// Stamp this resolved authentication claim data into the context/ctx flow
	claimBytes, err := json.Marshal(claim)
	if err != nil {
		return ctx, fmt.Errorf("error marshalling claim: %v", err)
	}

	err = internal.SetBaggage(ctx, u.Name(), destination, base64.StdEncoding.EncodeToString(claimBytes))
	return ctx, err
}

func (u *uberGalileo) AuthenticateIn(ctx context.Context, allowed ...interface{}) error {
	if u.isDisabled() {
		u.log.Warn("global disable set")
		return nil
	}

	var allowedEntities []string
	for _, e := range allowed {
		if e == nil {
			continue
		}
		switch v := e.(type) {
		case string:
			allowedEntities = append(allowedEntities, v)
		case []string:
			// Backwards compatibility with old calling convention
			allowedEntities = append(allowedEntities, v...)
		default:
			return fmt.Errorf("unexpected argument of type %T passed to AuthenticateIn: %v", e, e)
		}
	}

	if len(allowedEntities) == 0 {
		allowedEntities = u.allowedEntities
	}

	if err := u.doAuthenticateIn(ctx, allowedEntities); err != nil {
		return &authError{
			Reason:          err,
			AllowedEntities: allowedEntities,
		}
	}

	return nil
}

func (u *uberGalileo) doAuthenticateIn(ctx context.Context, allowed []string) error {
	if u.isDisabled() {
		u.log.Warn("global disable set")
		return nil
	}
	// Ensure the context has a span that can be examined for auth baggage and
	// decorated with logs.
	ctx, finishSpan := internal.EnsureSpan(ctx, u.tracer)
	defer finishSpan()
	span := opentracing.SpanFromContext(ctx)

	span.LogFields(
		opentracinglog.Float32(internal.TagInEnforcePercent, u.enforcePercentage),
	)

	// Value of allowedTag at the time this function ends will dictate what we
	// log for TagInAllowed. We'll assume the request was denied by default.
	allowedTag := internal.Denied
	defer func() {
		span.LogFields(opentracinglog.Int(internal.TagInAllowed, allowedTag))
	}()

	m := u.metrics.Tagged(map[string]string{
		"enforce_percentage": fmt.Sprintf("%f", u.enforcePercentage),
	})

	// we would normally switch through the various authorize types based on
	// the configured contexts.
	claim, err := GetClaim(ctx)
	if err != nil {
		if !shouldEnforce(u.enforcePercentage) {
			u.log.Debug("missing or invalid auth information and enforcePercentage < 1",
				zap.Error(err),
				zap.Float32("enforce_percentag", u.enforcePercentage),
			)
			allowedTag = internal.NotEnforced
			go m.Tagged(map[string]string{"allowed": strconv.Itoa(internal.NotEnforced)}).Counter("authorize").Inc(1)
			return nil
		}

		return errors.New("no auth baggage found")
	}

	// if there are no allowed groups, everything's an allowed group.
	if len(allowed) == 0 {
		allowed = claim.Claims
	} else {
		// If we accept wonka.EveryEntity, add the entity name on the remote claim.
		// This is just in case the remote side got an identity claim for their
		// entity name rather than one for wonka.EveryEntity.
		for _, c := range allowed {
			if strings.EqualFold(c, wonka.EveryEntity) {
				allowed = append(allowed, claim.EntityName)
				break
			}
		}
	}

	u.log.Debug("checking claim validity")

	span.LogFields(
		opentracinglog.String(internal.TagInEntityName, claim.EntityName),
		opentracinglog.String(internal.TagInDestination, claim.Destination),
	)

	if err := claim.Check(u.serviceName, allowed); err == nil {
		go m.Tagged(map[string]string{
			"allowed":       strconv.Itoa(internal.AllowedAllOK),
			"remote_entity": claim.EntityName,
		}).Counter("authorize").Inc(1)
		allowedTag = internal.AllowedAllOK
	} else {
		if shouldEnforce(u.enforcePercentage) {
			go m.Tagged(map[string]string{
				"allowed":            strconv.Itoa(internal.Denied),
				"enforce_percentage": fmt.Sprintf("%f", u.enforcePercentage),
				"remote_entity":      claim.EntityName,
			}).Counter("authorize").Inc(1)
			return fmt.Errorf("not permitted by configuration: %v", err)
		}

		allowedTag = internal.NotEnforced
		go m.Tagged(map[string]string{
			"allowed":            strconv.Itoa(internal.NotEnforced),
			"enforce_percentage": fmt.Sprintf("%f", u.enforcePercentage),
			"remote_entity":      claim.EntityName,
		}).Counter("authorize").Inc(1)
		u.log.Debug("not an allowed service but passed enforce_percentage",
			zap.String("remote_entity", claim.EntityName),
		)
	}

	u.log.Debug("successfully allowed")
	return nil
}

func (u *uberGalileo) resolveClaim(ctx context.Context, destination, claimReq string) (*wonka.Claim, error) {
	if claimReq != "" {
		return u.w.ClaimRequest(ctx, claimReq, destination)
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
	u.log.Debug("checking skip", zap.String("destination", entity))

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

// claimUsable returns nil if the given claim token can still be used, i.e. is
// valid, won't expire soon, and asserts claimReq. Returns an error otherwise.
func claimUsable(claim *wonka.Claim, claimReq string) error {
	if err := claim.Validate(); err != nil {
		return err
	}

	// We already know the token is not expired. Check if it will expire soon.
	expireTime := time.Unix(claim.ValidBefore, 0)
	now := time.Now()
	if now.Add(claimExpiryBuffer).After(expireTime) {
		return errors.New("claim token will expire soon")
	}

	// TODO(jkline): claimUsable is very close to claim.Check, but
	// claim.Check doesn't handle an empty slice of allowed claims.
	// We have a claim in our cache that matches the destination but if we
	// were given an explicit claim request, we should only reuse this claim
	// if the explictly requested claim is present.
	if claimReq == "" {
		// No explicit claim request, any token will do.
		return nil
	}
	for _, c := range claim.Claims {
		if strings.EqualFold(claimReq, c) {
			return nil
		}
	}

	return fmt.Errorf("claim token does not grant %q", claimReq)
}
