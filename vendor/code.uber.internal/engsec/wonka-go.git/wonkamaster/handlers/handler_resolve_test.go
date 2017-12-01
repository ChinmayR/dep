package handlers

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestResolve(t *testing.T) {
	var testVars = []struct {
		e1     string
		e1In   []string
		e1TTL  int64
		badReq bool

		e2         string
		e2Requires []string
		expect     []string
		err        string
	}{
		{e1: "test@uber.com", e2: "foober", expect: []string{wonka.EveryEntity, "test@uber.com"}},
		{e1: "test@uber.com", e2: "doober", e2Requires: []string{"foo"},
			expect: []string{wonka.EveryEntity, "test@uber.com"}},
		{e1: "test@uber.com", e1In: []string{"AD:foo"}, e2: "doober", e2Requires: []string{"AD:foo"},
			expect: []string{wonka.EveryEntity, "test@uber.com", "AD:foo"}},
		{e1: "test@uber.com", e1In: []string{"AD:foo"}, badReq: true,
			e2: "doober", e2Requires: []string{"AD:foo"}, err: "error"},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("%d: %+v", idx, m), func(t *testing.T) {
			wonkatestdata.WithUSSHAgent(m.e1, func(agentPath string, caKey ssh.PublicKey) {
				wonkatestdata.WithWonkaMaster("foober", func(r common.Router, handlerCfg common.HandlerConfig) {
					mem := make(map[string][]string)
					mem[m.e1] = m.e1In
					handlerCfg.Pullo = rpc.NewMockPulloClient(mem)

					if len(m.e2Requires) != 0 {
						entity := &wonka.Entity{
							EntityName: m.e2,
							Requires:   strings.Join(m.e2Requires, ","),
						}
						handlerCfg.DB.Create(context.TODO(), entity)
					}
					SetupHandlers(r, handlerCfg)

					a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
					require.NoError(t, err, "ssh-agent dial error: %v", err)

					w, err := wonka.Init(wonka.Config{EntityName: m.e1, Agent: agent.NewClient(a)})
					require.NoError(t, err, "error initializing wonka: %v", err)

					if m.badReq {
						os.Setenv("WONKA_USSH_CA", fmt.Sprintf("foo"))
					}

					c, err := w.ClaimResolve(context.Background(), m.e2)
					if m.err != "" {
						require.Error(t, err)
						require.Contains(t, err.Error(), "failed to load user ca file")
					} else {
						require.NoError(t, err, "shouldn't err: %v", err)

						sort.Strings(m.expect)
						claims := c.Claims
						sort.Strings(claims)
						require.Equal(t, m.expect, claims, "should be equal")
					}
				})
			})
		})
	}
}
