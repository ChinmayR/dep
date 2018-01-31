package claimtools

import (
	"context"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v2"

	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	"code.uber.internal/engsec/galileo-go.git/internal/testhelper"
	wonka "code.uber.internal/engsec/wonka-go.git"

	"github.com/stretchr/testify/assert"
)

func TestNewInboundCacheReturnsDisabled(t *testing.T) {
	cacheSize := 0
	cfg := CacheConfig{MaxSize: &cacheSize}
	ic, err := NewInboundCache(cfg)
	assert.NoError(t, err)
	assert.Equal(t, DisabledCache, ic)
}

func TestNewInboundCacheReturnsDefaultCacheSize(t *testing.T) {
	ic := newTestInboundCache(t)
	assert.Equal(t, false, ic.disabled)
	assert.Equal(t, 0, ic.cache.Len())
	for i := 0; i < DefaultCacheSize; i++ {
		ic.cache.Add(i, "foo")
	}
	assert.Equal(t, DefaultCacheSize, ic.cache.Len())
}

func newTestInboundCache(t *testing.T) *InboundCache {
	ic, err := NewInboundCache(CacheConfig{})
	assert.NoError(t, err)
	return ic
}

func ctxWithClaim(claimString string) context.Context {
	_, ctx, span := testhelper.SetupContext()
	contexthelper.SetBaggage(span, claimString)
	return ctx
}

func TestGetOrCreateFromContext(t *testing.T) {
	testhelper.WithSignedClaim(t, func(claim *wonka.Claim, claimString string) {
		ic := newTestInboundCache(t)
		ctx := ctxWithClaim(claimString)
		cc, err := ic.GetOrCreateFromContext(ctx)
		assert.NoError(t, err)
		assert.Equal(t, *claim, *cc.Claim)
	})
}

func TestParseClaimWithoutCachingFails(t *testing.T) {
	ic := newTestInboundCache(t)
	claimString := "badly formed claim"
	ctx := ctxWithClaim(claimString)
	cc, err := ic.parseClaimWithoutCaching(ctx)
	assert.Error(t, err)
	assert.Nil(t, cc)
}

func TestParseClaimWithoutCachingSucceeds(t *testing.T) {
	testhelper.WithSignedClaim(t, func(claim *wonka.Claim, claimString string) {
		ic := newTestInboundCache(t)
		ctx := ctxWithClaim(claimString)
		cc, err := ic.parseClaimWithoutCaching(ctx)
		assert.NoError(t, err)
		assert.Equal(t, *claim, *cc.Claim)
		assert.Equal(t, true, cc.cacheDisabled)
		assert.Nil(t, cc.cache)
	})
}

func TestGetCachedClaimNotExists(t *testing.T) {
	ic := newTestInboundCache(t)
	cc, ok := ic.getCachedClaim("badababadaba")
	assert.False(t, ok)
	assert.Nil(t, cc)
}

func TestCacheClaimBadDecode(t *testing.T) {
	ic := newTestInboundCache(t)
	rawToken := "bad claim"
	cc := ic.parseAndCacheClaim(rawToken)
	assert.Error(t, cc.ctxErr)
	cc, ok := ic.getCachedClaim(rawToken)
	assert.True(t, ok)
	assert.Error(t, cc.ctxErr)
}

func TestCacheClaimGood(t *testing.T) {
	ic := newTestInboundCache(t)
	rawToken := "eyJjdCI6IldPTktBQyIsInZhIjoxNTExMzY5MzAzLCJ2YiI6MTUxMTQ1NTc2MywiZSI6ImZvbyIsImMiOlsiZm9vIl0sImQiOiJiYXIiLCJzIjoiIn0="
	cc := ic.parseAndCacheClaim(rawToken)
	assert.NoError(t, cc.ctxErr)
	cc, ok := ic.getCachedClaim(rawToken)
	assert.True(t, ok)
	assert.NoError(t, cc.ctxErr)
}

func TestNewCacheKey(t *testing.T) {
	tests := []struct {
		serviceAliases  []string
		allowedServices []string
	}{
		{[]string{"me", "myself", "i"}, []string{"you", "your-friends", "others"}},
		{[]string{"myself", "i", "me"}, []string{"your-friends", "you", "others"}},
		{[]string{"i", "myself", "me"}, []string{"others", "your-friends", "you"}},
	}

	for _, tt := range tests {
		cacheKey := newCacheKey(tt.serviceAliases, tt.allowedServices)
		assert.Equal(t, "i,me,myself::others,you,your-friends", cacheKey)
	}
}

func TestInspect(t *testing.T) {
	dest := "me"
	testhelper.WithSignedClaim(t, func(claim *wonka.Claim, _ string) {
		cc := newCachedClaim(claim)
		err := cc.Inspect([]string{dest}, []string{wonka.EveryEntity})
		assert.NoError(t, err)

		// introspect to ensure that it ended up in the cache
		ok, err := cc.cache.Load("me::EVERYONE")
		assert.True(t, ok)
		assert.NoError(t, err)

		// strip out the underlying claim object and make sure we can still inspect
		cc.Claim = nil
		err = cc.Inspect([]string{dest}, []string{wonka.EveryEntity})
		assert.NoError(t, err)
	}, testhelper.Destination(dest))
}

func TestInspectDisabled(t *testing.T) {
	dest := "me"
	testhelper.WithSignedClaim(t, func(claim *wonka.Claim, _ string) {
		cc := newCachedClaim(claim)
		cc.cacheDisabled = true
		err := cc.Inspect([]string{dest}, []string{wonka.EveryEntity})
		assert.NoError(t, err)

		// introspect to ensure that it didn't end up in the cache
		ok, _ := cc.cache.Load("me::EVERYONE")
		assert.False(t, ok)
	}, testhelper.Destination(dest))
}

func TestInspectExpired(t *testing.T) {
	dest := "me"
	testhelper.WithSignedClaim(t, func(claim *wonka.Claim, _ string) {
		cc := newCachedClaim(claim)
		cc.expireAt = time.Unix(0, 0)
		cc.Claim.ValidBefore = 0
		err := cc.Inspect([]string{dest}, []string{wonka.EveryEntity})
		assert.Error(t, err)
	}, testhelper.Destination(dest))
}

func TestConfig(t *testing.T) {
	tests := []struct {
		configBytes   []byte
		expectMaxSize *int
		expectErrStr  string
	}{
		{
			configBytes:   []byte("max_size: 0"),
			expectMaxSize: intPtr(0),
		},
		{
			configBytes:   []byte("max_size: 100"),
			expectMaxSize: intPtr(100),
		},
		{
			configBytes:   []byte(""),
			expectMaxSize: nil,
		},
		{
			configBytes:   []byte("max_size:"),
			expectMaxSize: nil,
		},
		{
			configBytes:  []byte("max_size: abc"),
			expectErrStr: "unmarshal error",
		},
	}
	for _, tt := range tests {
		var cfg CacheConfig
		err := yaml.Unmarshal(tt.configBytes, &cfg)
		if tt.expectErrStr != "" {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectErrStr)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tt.expectMaxSize, cfg.MaxSize)
		}
	}
}

func intPtr(x int) *int { return &x }
