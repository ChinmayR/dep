package claimtools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

var (
	// DefaultCacheSize is the maximum number of claims that will be cached.
	DefaultCacheSize = 4096

	// DisabledCache is an InboundCache without caching.
	DisabledCache = &InboundCache{disabled: true}
)

// CacheConfig configures how the cache operates.
type CacheConfig struct {
	// MaxSize sets the capacity of inbound claim caching (default: 4096).
	// A value of `0` results in caching being disabled.
	MaxSize *int `yaml:"max_size"`
}

// InboundCache is a cache for storing the verification results of claims received
// on inbound authentication requests.
type InboundCache struct {
	cache    *lru.Cache
	disabled bool
}

// NewInboundCache returns a new cache for recalling results of inbound requests.
func NewInboundCache(cfg CacheConfig) (*InboundCache, error) {
	cacheSize := DefaultCacheSize
	if cfg.MaxSize != nil {
		cacheSize = *cfg.MaxSize
	}
	if cacheSize <= 0 {
		return DisabledCache, nil
	}
	cache, err := lru.New(cacheSize)
	if err != nil {
		return nil, fmt.Errorf("error creating inbound claim cache: %v", err)
	}
	return &InboundCache{
		cache: cache,
	}, nil
}

// GetOrCreateFromContext looks at the context to either:
//	1. Get a claim from the cache that matches the one attached to ctx; or
//	2. Create a new claim in the cache using the claim pulled from ctx.
func (ic *InboundCache) GetOrCreateFromContext(ctx context.Context) (*CacheableClaim, internal.InboundAuthenticationError) {
	if ic.disabled {
		return ic.parseClaimWithoutCaching(ctx)
	}
	rawToken, err := contexthelper.StripTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if cachedClaim, ok := ic.getCachedClaim(rawToken); ok {
		return cachedClaim, cachedClaim.ctxErr
	}
	cachedClaim := ic.parseAndCacheClaim(rawToken)
	return cachedClaim, cachedClaim.ctxErr
}

func (ic *InboundCache) parseClaimWithoutCaching(ctx context.Context) (*CacheableClaim, internal.InboundAuthenticationError) {
	claim, err := contexthelper.ClaimFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return &CacheableClaim{
		Claim:         claim,
		cacheDisabled: true,
	}, nil
}

func (ic *InboundCache) getCachedClaim(rawToken string) (*CacheableClaim, bool) {
	val, ok := ic.cache.Get(rawToken)
	if !ok {
		return nil, false
	}
	return val.(*CacheableClaim), true
}

func (ic *InboundCache) parseAndCacheClaim(rawToken string) *CacheableClaim {
	claim, err := contexthelper.UnmarshalToken(rawToken)
	if err != nil {
		cachedClaim := &CacheableClaim{ctxErr: err}
		ic.cache.Add(rawToken, cachedClaim)
		return cachedClaim
	}
	cachedClaim := newCachedClaim(claim)
	ic.cache.Add(rawToken, cachedClaim)
	return cachedClaim
}

// CacheableClaim is a wonka Claim whose calls to Inspect are cached.
type CacheableClaim struct {
	*wonka.Claim

	// cached map of <allowed entities> => <errors returned from claim.Inspect>
	cache         *syncMapStringError
	ctxErr        internal.InboundAuthenticationError
	expireAt      time.Time
	cacheDisabled bool
}

func newCachedClaim(claim *wonka.Claim) *CacheableClaim {
	return &CacheableClaim{
		Claim:    claim,
		cache:    newSyncMapStringError(),
		expireAt: time.Unix(claim.ValidBefore, 0),
	}
}

// NewCachedClaimDisabled is a temporary workaround for validatecredential methods.
// TODO(tjulian): remove this when validatecredential has caching.
func NewCachedClaimDisabled(claim *wonka.Claim) *CacheableClaim {
	return &CacheableClaim{
		Claim:         claim,
		cacheDisabled: true,
	}
}

// Inspect verifies that the CacheableClaim is valid for the serviceName and contains
// claims for the allowed entities.
func (cr *CacheableClaim) Inspect(serviceNames, allowed []string) error {
	if cr.cacheDisabled {
		return cr.Claim.Inspect(serviceNames, allowed)
	}
	// don't use the cached result if the claim appears to be expired
	if now := time.Now(); now.After(cr.expireAt) {
		return cr.Claim.Inspect(serviceNames, allowed)
	}
	cacheKey := newCacheKey(serviceNames, allowed)
	if ok, err := cr.cache.Load(cacheKey); ok {
		return err
	}
	err := cr.Claim.Inspect(serviceNames, allowed)
	cr.cache.Store(cacheKey, err)
	return err
}

func newCacheKey(serviceNames, allowed []string) string {
	sort.Strings(allowed)
	sort.Strings(serviceNames)
	return fmt.Sprintf("%s::%s", strings.Join(serviceNames, ","), strings.Join(allowed, ","))
}
