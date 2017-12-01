package wonka_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestImplicitClaims(t *testing.T) {
	var testVars = []struct {
		impClaims []string
		grps      []string

		claims []string
	}{
		{impClaims: []string{"AD:grp1", "AD:grp2"}, grps: []string{"AD:grp1", "AD:grp2"}, claims: []string{"EVERYONE", "AD:grp1", "AD:grp2"}},
		{impClaims: []string{"AD:grp1", "AD:grp2"}, grps: []string{"AD:grp1"}, claims: []string{"EVERYONE", "AD:grp1"}},
		{impClaims: []string{"AD:grp1", "AD:grp2"}, claims: []string{"EVERYONE"}},
	}

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("implicit_claims_%d", idx), func(t *testing.T) {
			wonkatestdata.WithUSSHAgent("foo@uber.com", func(agentPath string, caKey ssh.PublicKey) {
				wonkatestdata.WithWonkaMaster("foo", func(r common.Router, cfg common.HandlerConfig) {
					mem := make(map[string][]string)
					mem["foo@uber.com"] = m.grps
					cfg.Pullo = rpc.NewMockPulloClient(mem)
					handlers.SetupHandlers(r, cfg)

					w, err := wonka.Init(wonka.Config{EntityName: "foo@uber.com", ImplicitClaims: m.impClaims})
					require.NoError(t, err)
					c, err := w.ClaimRequest(context.Background(), wonka.EveryEntity, "me")
					require.NoError(t, err)

					sort.Strings(m.claims)
					sort.Strings(c.Claims)
					require.Equal(t, m.claims, c.Claims)
				})
			})
		})
	}
}

func TestClaimInspect(t *testing.T) {
	setupWonka(t, func(alice, bob wonka.Wonka) {
		claim, err := alice.ClaimRequest(context.Background(), wonka.EveryEntity, alice.EntityName())
		require.NoError(t, err, "claim request error: %v", err)

		t.Run("same destination", func(t *testing.T) {
			// claim is from alice, for alice. So this test
			// is bob saying, "is this an EVERYONE claim good for alice?"
			err := claim.Inspect([]string{alice.EntityName()}, []string{wonka.EveryEntity})
			require.NoError(t, err, "bob inspecting claim for alice should succeed")
		})

		t.Run("different destination", func(t *testing.T) {
			// claim is from alice, for alice. So this test
			// is bob saying, "is this an EVERYONE claim good for bob?"
			err := claim.Inspect([]string{bob.EntityName()}, []string{wonka.EveryEntity})
			require.Error(t, err, "bob inspecting claim for bob should fail")
		})
	})
}

func TestClaimCheck(t *testing.T) {
	setupWonka(t, func(alice, _ wonka.Wonka) {
		claim, err := alice.ClaimRequest(context.Background(), wonka.EveryEntity, alice.EntityName())
		require.NoError(t, err, "claim request error: %v", err)

		t.Run("claim should check out", func(t *testing.T) {
			err := claim.Check(alice.EntityName(), []string{wonka.EveryEntity})
			require.NoError(t, err, "claim should check out")
		})

		t.Run("no allowed claims", func(t *testing.T) {
			err := claim.Check(alice.EntityName(), []string{"OTHER"})
			require.Error(t, err, "claim should not check out")
		})

		t.Run("different desination", func(t *testing.T) {
			err := claim.Check("bob-service", []string{wonka.EveryEntity})
			require.Error(t, err, "claim should not check out")
		})
	})
}

func TestResolveEnrolled(t *testing.T) {
	setupWonka(t, func(alice, bob wonka.Wonka) {
		claim, err := alice.ClaimResolve(context.Background(), bob.EntityName())
		require.NoError(t, err, "claim request error: %v", err)
		require.NotNil(t, claim)

		exp := []string{wonka.EveryEntity, alice.EntityName()}
		sort.Strings(exp)
		sort.Strings(claim.Claims)
		require.Equal(t, exp, claim.Claims)
	})
}

// For this test the ussh user cert, wonka entity, and claim are all valid
// personnel name, i.e email address.
func TestResolveUssh(t *testing.T) {
	personnelEntityName := "alice@example.com"
	wonkatestdata.WithUSSHAgent(personnelEntityName, func(agentPath string, caKey ssh.PublicKey) {
		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			a, err := net.Dial("unix", agentPath)
			require.NoError(t, err, "ssh-agent dial error: %v", err)

			w, err := wonka.Init(wonka.Config{EntityName: personnelEntityName, Agent: agent.NewClient(a)})
			require.NoError(t, err, "error initializing wonka: %v", err)

			c, err := w.ClaimResolve(context.Background(), "bob-service")
			require.NoError(t, err)

			exp := []string{wonka.EveryEntity, personnelEntityName}
			sort.Strings(exp)
			sort.Strings(c.Claims)
			require.Equal(t, exp, c.Claims)
		})
	})
}

// For this test the user eve@example.com has a ussh user cert, and tries to get
// a claim affirming she is alice@example.com.
func TestResolveUsshDifferentPersonnelFails(t *testing.T) {
	wonkatestdata.WithUSSHAgent("eve@example.com", func(agentPath string, caKey ssh.PublicKey) {
		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
			require.NoError(t, err, "ssh-agent dial error: %v", err)
			w, err := wonka.Init(wonka.Config{EntityName: "alice@example.com", Agent: agent.NewClient(a)})
			require.NoError(t, err, "error initializing wonka: %v", err)
			require.Equal(t, "eve@example.com", w.EntityName(), "wonka should have re-written the entity name")

			c, err := w.ClaimResolve(context.Background(), "bob-service")
			require.NoError(t, err, "error requesting a claim: %v", err)

			exp := []string{w.EntityName(), wonka.EveryEntity}
			sort.Strings(exp)
			sort.Strings(c.Claims)
			require.Equal(t, exp, c.Claims)
		})
	})
}

// For this test personnel user eve@example.com has a ussh uer cert, but
// configures a wonka client to request claims as some example
// service.
func TestResolveUsshNonPersonnelFails(t *testing.T) {
	wonkatestdata.WithUSSHAgent("eve@example.com", func(agentPath string, caKey ssh.PublicKey) {
		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
			require.NoError(t, err, "ssh-agent dial error: %v", err)
			w, err := wonka.Init(wonka.Config{EntityName: "wonkaSample:some-service", Agent: agent.NewClient(a)})
			require.NoError(t, err, "error initializing wonka: %v", err)
			require.Equal(t, "eve@example.com", w.EntityName(), "wonka should have re-written the entity name")

			c, err := w.ClaimResolve(context.Background(), "bob-service")
			require.NoError(t, err, "error requesting a claim: %v", err)

			exp := []string{w.EntityName(), wonka.EveryEntity}
			sort.Strings(exp)
			sort.Strings(c.Claims)
			require.Equal(t, exp, c.Claims)
		})
	})
}

func TestClaimMultipleKeys(t *testing.T) {
	c := wonka.Claim{
		ClaimType:   "foo",
		ValidAfter:  time.Now().Add(-time.Minute).Unix(),
		ValidBefore: time.Now().Add(time.Minute).Unix(),
		Claims:      []string{wonka.EveryEntity},
		Destination: "foo",
	}
	toSign, err := json.Marshal(c)
	require.NoError(t, err, "error marshalling to sign: %v", err)

	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating key: %v", err)

	k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "error generating key: %v", err)

	sig, err := wonkacrypter.New().Sign(toSign, k1)
	require.NoError(t, err, "error signing: %v", err)
	c.Signature = sig

	oldWonkaMasterPubkeys := wonka.WonkaMasterPublicKeys
	defer func() { wonka.WonkaMasterPublicKeys = oldWonkaMasterPubkeys }()
	wonka.WonkaMasterPublicKeys = []*ecdsa.PublicKey{&k2.PublicKey}

	err = c.Validate()
	require.Error(t, err, "claim shouldn't be valid")

	wonka.WonkaMasterPublicKeys = append(wonka.WonkaMasterPublicKeys, &k1.PublicKey)

	err = c.Validate()
	require.NoError(t, err, "claim should be valid")
}

func TestMarshalClaim(t *testing.T) {
	c1 := &wonka.Claim{EntityName: "foo"}
	c1Bytes, err := wonka.MarshalClaim(c1)
	require.NoError(t, err)

	_, err = wonka.UnmarshalClaim(c1Bytes)
	require.NoError(t, err)
}

func BenchmarkClaimRequest(b *testing.B) {
	defer zap.ReplaceGlobals(zap.NewNop())()
	setupWonka(b, func(alice, _ wonka.Wonka) {
		for i := 0; i < b.N; i++ {
			claim, err := alice.ClaimRequest(context.Background(), wonka.EveryEntity, alice.EntityName())
			require.NoError(b, err, "claim request shouldn't err: %v", err)
			require.Equal(b, claim.Claims, []string{wonka.EveryEntity}, "should be everyone claim")
			require.Equal(b, claim.Destination, alice.EntityName(), "claim should be for %s", alice.EntityName())
		}
	})
}

func BenchmarkClaimVerify(b *testing.B) {
	defer zap.ReplaceGlobals(zap.NewNop())()
	setupWonka(b, func(alice, _ wonka.Wonka) {
		claim, err := alice.ClaimRequest(context.Background(), wonka.EveryEntity, alice.EntityName())
		if err != nil {
			panic(err)
		}

		for i := 0; i < b.N; i++ {
			if err := claim.Check(alice.EntityName(), []string{wonka.EveryEntity}); err != nil {
				panic(err)
			}
		}
	})
}
