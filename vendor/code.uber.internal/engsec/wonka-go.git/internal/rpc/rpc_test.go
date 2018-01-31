package rpc_test

import (
	"context"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var _defaultGroups = map[string]struct{}{
	wonka.EveryEntity: {},
}

func TestNewPulloClientWithOpts(t *testing.T) {
	p, err := rpc.NewPulloClient(
		rpc.Logger(zap.NewNop(), zap.NewAtomicLevel()),
		rpc.MuttleyHostPort("127.0.0.1:101"),
	)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestGetGroupsForUserReturnsDefaultWhenTheChannelFailsToConnectToPullo(t *testing.T) {
	p, err := rpc.NewPulloClient(
		rpc.Logger(zap.NewNop(), zap.NewAtomicLevel()),
		rpc.MuttleyHostPort("127.0.0.1:101"),
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	res, err := p.GetGroupsForUser(context.Background(), "should be default")
	assert.Equal(t, _defaultGroups, res, "expected a failed call to pullo")
	assert.Error(t, err)
}

func TestIsMemberOfDefaultsToFalseWhenTheChannelFailsToConnectToPullo(t *testing.T) {
	p, err := rpc.NewPulloClient(
		rpc.Logger(zap.NewNop(), zap.NewAtomicLevel()),
		rpc.MuttleyHostPort("127.0.0.1:101"),
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	res, err := p.IsMemberOf(context.Background(), "cmcandre@uber.com", "engineering")
	assert.False(t, res, "expected a failed call to pullo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestPulloClient(t *testing.T) {
	membership := map[string][]string{
		"professorx@example.com": {"AD:x-men"},
		"wolverine@example.com":  {"AD:x-men", "AD:avengers"},
		"ironman@example.com":    {"AD:avengers"},
		"thor@example.com":       {"avengers"},
	}

	// Mock client shares logic with real client.
	p := rpc.NewMockPulloClient(
		membership,
		rpc.Logger(zap.NewNop(), zap.NewAtomicLevel()),
	)

	ctx := context.Background()

	t.Run("one group", func(t *testing.T) {
		expected := map[string]struct{}{
			wonka.EveryEntity: {},
			"ad:x-men":        {},
		}
		groups, err := p.GetGroupsForUser(ctx, "professorx@example.com")
		assert.NoError(t, err, "get groups for user should succeed")
		assert.Equal(t, expected, groups, "unexpected group membership")

		isMember, err := p.IsMemberOf(ctx, "professorx@example.com", "x-men")
		assert.NoError(t, err, "membership check should succeed")
		assert.True(t, isMember, "Professor X is an x-man")

		isMember, err = p.IsMemberOf(ctx, "professorx@example.com", "avengers")
		assert.NoError(t, err, "membership check should succeed")
		assert.False(t, isMember, "Professor X is not an avenger")
	})

	t.Run("multiple groups", func(t *testing.T) {
		expected := map[string]struct{}{
			wonka.EveryEntity: {},
			"ad:x-men":        {},
			"ad:avengers":     {},
		}
		groups, err := p.GetGroupsForUser(ctx, "wolverine@example.com")
		assert.NoError(t, err, "get groups for user should succeed")
		assert.Equal(t, expected, groups, "unexpected group membership")

		isMember, err := p.IsMemberOf(ctx, "wolverine@example.com", "x-men")
		assert.NoError(t, err, "membership check should succeed")
		assert.True(t, isMember, "wolverine is an x-man")

		isMember, err = p.IsMemberOf(ctx, "wolverine@example.com", "avengers")
		assert.NoError(t, err, "membership check should succeed")
		assert.True(t, isMember, "wolverine is an avenger")
	})

	t.Run("zero groups", func(t *testing.T) {
		groups, err := p.GetGroupsForUser(ctx, "superman@example.com")
		assert.NoError(t, err, "get groups for user should succeed")
		assert.Equal(t, _defaultGroups, groups, "groups should be nil")

		isMember, err := p.IsMemberOf(ctx, "superman@example.com", "x-men")
		assert.NoError(t, err, "membership check should succeed")
		assert.False(t, isMember, "superman is not an x-man")
	})

	t.Run("prefix optional in members list", func(t *testing.T) {
		// members list for Thor does not use AD: prefix.
		assert.Equal(t, []string{"avengers"}, membership["thor@example.com"], "test is set up incorrectly")

		expected := map[string]struct{}{
			wonka.EveryEntity: {},
			"ad:avengers":     {},
		}
		groups, err := p.GetGroupsForUser(ctx, "thor@example.com")
		assert.NoError(t, err, "get groups for user should succeed")
		assert.Equal(t, expected, groups, "unexpected group membership")

		// Membership checks may use AD: prefix, at least for MockPullo.
		isMember, err := p.IsMemberOf(ctx, "thor@example.com", "ad:avengers")
		assert.NoError(t, err, "membership check should succeed")
		assert.True(t, isMember, "thor is an avenger")
	})

	t.Run("prefix allowed in membership check", func(t *testing.T) {
		// Membership checks may use AD: prefix, at least for MockPullo.
		isMember, err := p.IsMemberOf(ctx, "ironman@example.com", "ad:avengers")
		assert.NoError(t, err, "membership check should succeed")
		assert.True(t, isMember, "ironman is an avenger")
	})
}
