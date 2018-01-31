package rpc

import (
	"strings"

	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"
)

type mockPullo struct {
	members map[string][]string
	log     *zap.Logger
}

// NewMockPulloClient creates a new mock pullo client
func NewMockPulloClient(members map[string][]string, opts ...PulloClientOption) PulloClient {
	cfg := pulloClientConfig{
		Logger: zap.L(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	client := mockPullo{members: members, log: cfg.Logger}
	opts = append(opts, Client(client))
	pc, _ := NewPulloClient(opts...)
	return pc
}

// GetUserGroups implements TChanPullo and returns our mock memberships
func (m mockPullo) GetUserGroups(_ thrift.Context, user string) ([]string, error) {
	m.log.Debug("pullo membership request", zap.String("user", user))
	if grps, ok := m.members[user]; ok {
		return grps, nil
	}
	return nil, nil
}

// IsMemberOf implements TChanPullo and returns our mock memberships
func (m mockPullo) IsMemberOf(_ thrift.Context, user, group string) (bool, error) {
	user = strings.ToLower(user)
	group = CanonicalGroupName(group)

	m.log.Debug("pullo membership request",
		zap.String("user", user),
		zap.String("group", group),
	)

	groups, ok := m.members[user]
	if !ok {
		m.log.Debug("user not found",
			zap.String("user", user),
			zap.String("group", group),
		)

		return false, nil
	}

	for _, g := range groups {
		if group == CanonicalGroupName(g) {
			m.log.Debug("user found in group",
				zap.Any("user", user),
				zap.Any("group", group),
			)

			return true, nil
		}
	}

	m.log.Debug("user not in group",
		zap.Any("user", user),
		zap.Any("group", group),
		zap.Any("groups", groups),
	)

	return false, nil
}
