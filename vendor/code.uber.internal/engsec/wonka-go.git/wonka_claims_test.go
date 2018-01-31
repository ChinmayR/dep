package wonka_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestGetInvalidityReason(t *testing.T) {
	t.Run("validation error", func(t *testing.T) {
		reason, ok := wonka.GetInvalidityReason(
			internal.NewValidationErrorf(internal.InvalidFromFuture, "time traveling error"),
		)
		assert.Equal(t, string(internal.InvalidFromFuture), reason, "unexpected reason")
		assert.True(t, ok, "ok should be true")
	})

	t.Run("other error type", func(t *testing.T) {
		reason, ok := wonka.GetInvalidityReason(errors.New("foobar"))
		assert.Equal(t, "", reason, "reason should be empty")
		assert.False(t, ok, "ok should be false")
	})

	t.Run("nil", func(t *testing.T) {
		reason, ok := wonka.GetInvalidityReason(nil)
		assert.Equal(t, "", reason, "reason should be empty")
		assert.False(t, ok, "ok should be false")
	})

	t.Run("nil validation error", func(t *testing.T) {
		var err *internal.ValidationError
		reason, ok := wonka.GetInvalidityReason(err)
		assert.Equal(t, "", reason, "reason should be empty")
		assert.False(t, ok, "ok should be false")
	})

	t.Run("empty validation error", func(t *testing.T) {
		var err internal.ValidationError
		reason, ok := wonka.GetInvalidityReason(&err)
		assert.Equal(t, "", reason, "reason should be empty")
		assert.False(t, ok, "ok should be false")
	})
}

func TestImplicitClaims(t *testing.T) {
	var testVars = []struct {
		impClaims []string
		grps      []string

		claims []string
	}{
		{impClaims: []string{"AD:grp1", "AD:grp2"}, grps: []string{"AD:grp1", "AD:grp2"},
			claims: []string{"foo@uber.com", "EVERYONE", "AD:grp1", "AD:grp2"}},
		{impClaims: []string{"AD:grp1", "AD:grp2"}, grps: []string{"AD:grp1"},
			claims: []string{"foo@uber.com", "EVERYONE", "AD:grp1"}},
		{impClaims: []string{"AD:grp1", "AD:grp2"}, claims: []string{"foo@uber.com", "EVERYONE"}},
	}

	name := "foo@uber.com"

	for idx, m := range testVars {
		t.Run(fmt.Sprintf("implicit_claims_%d", idx), func(t *testing.T) {
			wonkatestdata.WithUSSHAgent(name, func(agentPath string, caKey ssh.PublicKey) {
				wonkatestdata.WithWonkaMaster("", func(r common.Router, cfg common.HandlerConfig) {
					mem := make(map[string][]string)
					mem[name] = m.grps
					cfg.Pullo = rpc.NewMockPulloClient(mem,
						rpc.Logger(cfg.Logger, zap.NewAtomicLevel()))
					cfg.Ussh = []ssh.PublicKey{caKey}
					handlers.SetupHandlers(r, cfg)

					w, err := wonka.Init(wonka.Config{EntityName: name, ImplicitClaims: m.impClaims})
					require.NoError(t, err, "error initializing wonka")
					c, err := w.ClaimRequest(context.Background(), wonka.EveryEntity, "some-destination-service")
					require.NoError(t, err, "claim request should succeed")

					// Machinations for case insensitive slice comparison.
					for idx, item := range m.claims {
						m.claims[idx] = strings.ToLower(item)
					}
					for idx, item := range c.Claims {
						c.Claims[idx] = strings.ToLower(item)
					}

					sort.Strings(m.claims)
					sort.Strings(c.Claims)
					require.Equal(t, m.claims, c.Claims, "token does not affirm expected claims")
				})
			})
		})
	}
}

func TestClaimInspect(t *testing.T) {
	var testVars = []struct {
		destination string
		errMsg      string
	}{
		{
			destination: "first-allowed-destination",
		},
		{
			destination: "second-allowed-destination",
		},
		{
			destination: "unacceptable-destination",
			// Make sure failure is destination related, and not some other
			// claim validation problem.
			errMsg: `claim token destination "unacceptable-destination" is not among allowed destinations ["first-allowed-destination" "second-allowed-destination"]`,
		},
	}

	setupWonka(t, func(alice, _ wonka.Wonka) {
		for _, m := range testVars {
			t.Run(m.destination, func(t *testing.T) {

				// Claim is from alice
				claim, err := alice.ClaimRequest(context.Background(), wonka.EveryEntity, m.destination)
				require.NoError(t, err, "claim request error")

				err = claim.Inspect(
					[]string{"first-allowed-destination", "second-allowed-destination"},
					[]string{wonka.EveryEntity}, // claims
				)
				if m.errMsg == "" {
					require.NoError(t, err, "claim destined for %q should be allowed", m.destination)
				} else {
					require.Error(t, err, "claim destined for %q should be rejected", m.destination)
					assert.EqualError(t, err, m.errMsg, "unexpected error message")
				}
			})
		}

		t.Run("empty allowed destinations", func(t *testing.T) {
			// No token can possibly be valid when set of allowed destinations is empty.
			expectedErrMsg := `claim token destination "any-destination" is not among allowed destinations []`

			claim, err := alice.ClaimRequest(context.Background(), wonka.EveryEntity, "any-destination")
			require.NoError(t, err, "claim request error")

			err = claim.Inspect([]string{ /* empty allowed destinations */ }, []string{wonka.EveryEntity})

			require.Error(t, err, "claim destined for %q should be rejected", claim.Destination)
			assert.EqualError(t, err, expectedErrMsg, "unexpected error message")

			reason, ok := wonka.GetInvalidityReason(err)
			assert.True(t, ok, "type should be ValidationError")
			assert.Equal(t, string(internal.InvalidWrongDestination), reason, "unexpected error")
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

		t.Run("no common claims", func(t *testing.T) {
			err := claim.Check(alice.EntityName(), []string{"OTHER"})
			require.Error(t, err, "claim should not check out")
			reason, ok := wonka.GetInvalidityReason(err)
			assert.True(t, ok, "type should be ValidationError")
			assert.Equal(t, string(internal.InvalidNoCommonClaims), reason, "unexpected error")
		})

		t.Run("empty allowed claims", func(t *testing.T) {
			// No token can possibly be valid when set of allowed claims is empty.
			err := claim.Check(alice.EntityName(), []string{ /* empty allowed claims */ })
			require.Error(t, err, "claim should not check out")
			reason, ok := wonka.GetInvalidityReason(err)
			assert.True(t, ok, "type should be ValidationError")
			assert.Equal(t, string(internal.InvalidNoCommonClaims), reason, "unexpected error")
		})

		t.Run("different destination", func(t *testing.T) {
			err := claim.Check("bob-service", []string{wonka.EveryEntity})
			require.Error(t, err, "claim should not check out")
			reason, ok := wonka.GetInvalidityReason(err)
			assert.True(t, ok, "type should be ValidationError")
			assert.Equal(t, string(internal.InvalidWrongDestination), reason, "unexpected error")
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
			handlerCfg.Ussh = []ssh.PublicKey{caKey}
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
			handlerCfg.Ussh = []ssh.PublicKey{caKey}
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
			handlerCfg.Ussh = []ssh.PublicKey{caKey}
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

func TestClaimImpersonate(t *testing.T) {
	setupWonka(t, func(alice, bob wonka.Wonka) {
		claim, err := alice.ClaimImpersonateTTL(context.Background(), bob.EntityName(), wonka.EveryEntity, time.Hour)
		require.NoError(t, err, "claim request shouldn't err: %v", err)
		require.Equal(t, []string{wonka.EveryEntity, bob.EntityName()}, claim.Claims, "should be everyone/bob claim")
		require.Equal(t, wonka.EveryEntity, claim.Destination, "claim should be for %s", alice.EntityName())
	})
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

// setupWonka sets up a wonkamaster instance and enrolls two entities, alice and bob,
// suitable for communicating with each other like any two wonka entities.
func setupWonka(t testing.TB, fn func(alice, bob wonka.Wonka)) {
	os.Unsetenv("SSH_AUTH_SOCK")
	testdata.WithTempDir(func(dir string) {
		alicePrivPem := path.Join(dir, "alice.private.pem")
		aliceK := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		err := testdata.WritePrivateKey(aliceK, alicePrivPem)
		require.NoError(t, err, "error writing alice private %v", err)

		bobPrivPem := path.Join(dir, "bob.private.pem")
		bobK := testdata.PrivateKeyFromPem(testdata.RSAPriv2)
		err = testdata.WritePrivateKey(bobK, bobPrivPem)
		require.NoError(t, err, "error writing bob private %v", err)

		wonkatestdata.WithWonkaMaster("wonkaSample:test", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlerCfg.Imp = []string{"wonkaSample:alice"}
			handlers.SetupHandlers(r, handlerCfg)
			ctx := context.TODO()

			aliceEntity := wonka.Entity{
				EntityName:   "wonkaSample:alice",
				PublicKey:    string(testdata.PublicPemFromKey(aliceK)),
				ECCPublicKey: testdata.ECCPublicFromPrivateKey(aliceK),
			}
			err := handlerCfg.DB.Create(ctx, &aliceEntity)
			require.NoError(t, err, "create alice failed")

			aliceCfg := wonka.Config{
				EntityName:     "wonkaSample:alice",
				PrivateKeyPath: alicePrivPem,
			}
			alice, err := wonka.Init(aliceCfg)
			require.NoError(t, err, "alice wonka init error: %v", err)

			bobEntity := wonka.Entity{
				EntityName:   "wonkaSample:bob",
				PublicKey:    string(testdata.PublicPemFromKey(bobK)),
				ECCPublicKey: testdata.ECCPublicFromPrivateKey(bobK),
			}
			err = handlerCfg.DB.Create(ctx, &bobEntity)
			require.NoError(t, err, "create bob failed")

			bobCfg := wonka.Config{
				EntityName:     "wonkaSample:bob",
				PrivateKeyPath: bobPrivPem,
			}
			bob, err := wonka.Init(bobCfg)
			require.NoError(t, err, "bob wonka init error: %v", err)

			// now run our test
			fn(alice, bob)

			// cleanup
			handlerCfg.DB.Delete(ctx, aliceEntity.Name())
			handlerCfg.DB.Delete(ctx, bobEntity.Name())
		})
	})
}
