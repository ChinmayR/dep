package rpc

import (
	"fmt"

	"go.uber.org/zap"
)

type mockPullo struct {
	members map[string][]string
	log     *zap.Logger
}

// NewMockPulloClient creates a new mock pullo client
func NewMockPulloClient(members map[string][]string) PulloClient {
	// TODO(abg): Inject logger here
	return mockPullo{members: members, log: zap.L()}
}

// GetGroupsForUser returns our mock memberships
func (m mockPullo) GetGroupsForUser(user string) []string {
	m.log.Debug("pullo membership request", zap.Any("user", user))
	if grps, ok := m.members[user]; ok {
		return grps
	}
	return nil
}

func (m mockPullo) IsMemberOf(user, group string) bool {
	m.log.Debug("pullo membership request",
		zap.Any("user", user),
		zap.Any("group", group),
	)

	groups, ok := m.members[user]
	if !ok {
		m.log.Debug("user not found",
			zap.Any("user", user),
			zap.Any("group", group),
		)

		return false
	}

	for _, g := range groups {
		if fmt.Sprintf("AD:%s", group) == g {
			m.log.Debug("user found in group",
				zap.Any("user", user),
				zap.Any("group", group),
			)

			return true
		}
	}

	m.log.Debug("user not in group",
		zap.Any("user", user),
		zap.Any("group", group),
		zap.Any("groups", groups),
	)

	return false
}
