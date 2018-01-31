package yabplugin

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"github.com/yarpc/yab/transport"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func setEnvWithCleanup(key, value string) func() {
	prevValue := os.Getenv(key)
	os.Setenv(key, value)
	return func() {
		os.Setenv(key, prevValue)
	}
}

func setUserCAWithCleanup(newCA string) func() {
	prevCA := userCA
	userCA = newCA
	return func() {
		userCA = prevCA
	}
}

func setWonkamasterURLWithCleanup(newURL string) func() {
	prevURL := wonkamasterTestURL
	wonkamasterTestURL = newURL
	return func() {
		wonkamasterTestURL = prevURL
	}
}

// helper function to remove boilerplate for ussh agent and wonkamaster setup
func setupWonka(user string, fn func()) {
	wonkatestdata.WithUSSHAgent(user, func(agentPath string, caKey ssh.PublicKey) {
		wonkatestdata.WithWonkaMaster(user, func(r common.Router, handlerCfg common.HandlerConfig) {
			wonkatestdata.WithTempDir(func(dir string) {
				mockWonkamasterHealth(func() {
					handlerCfg.Ussh = []ssh.PublicKey{caKey}
					handlers.SetupHandlers(r, handlerCfg)
					caFile := path.Join(dir, "trusted_user_ca")
					err := ioutil.WriteFile(caFile, ssh.MarshalAuthorizedKey(caKey), os.ModePerm)
					if err != nil {
						panic(err)
					}
					defer setUserCAWithCleanup(caFile)()
					fn()
				})
			})
		})
	})
}

func mockWonkamasterHealth(fn func()) {
	handler := func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("OK")) }
	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()
	defer setWonkamasterURLWithCleanup(s.URL)()
	fn()
}

func TestYabMiddlewareSuccess(t *testing.T) {
	setupWonka("user@uber.com", func() {
		ri := NewRequestInterceptor(zap.L(), nil /* opts */)
		req, err := ri.Apply(context.TODO(), &transport.Request{TargetService: "foo"})
		require.NoError(t, err)

		claim, err := wonka.UnmarshalClaim(req.Baggage[contexthelper.ServiceAuthBaggageAttr])
		require.NoError(t, err)
		err = claim.Validate()
		require.NoError(t, err)
	})
}

func TestYabDisabled(t *testing.T) {
	defer setEnvWithCleanup("SSH_AUTH_SOCK", "")()
	ri := galileoRequestInterceptor{opts: &GalileoOpts{Disabled: true}}
	_, err := ri.Apply(context.TODO(), &transport.Request{})
	require.NoError(t, err)
}

func TestYabDisabledMessageNoWonkamaster(t *testing.T) {
	ri := galileoRequestInterceptor{opts: &GalileoOpts{}}
	_, err := ri.Apply(context.TODO(), &transport.Request{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--disable-galileo")
	require.Contains(t, err.Error(), "Error connecting to wonkamaster")
}

func TestYabDisabledMessageNoUsshCert(t *testing.T) {
	mockWonkamasterHealth(func() {
		defer setEnvWithCleanup("UBER_OWNER", "user@uber.com")()
		defer setEnvWithCleanup("SSH_AUTH_SOCK", "")()
		ri := galileoRequestInterceptor{opts: &GalileoOpts{}}
		_, err := ri.Apply(context.TODO(), &transport.Request{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--disable-galileo")
		require.Contains(t, err.Error(), "Error getting user ussh cert")
	})
}

func TestYabDisabledMessageNoUberOwner(t *testing.T) {
	mockWonkamasterHealth(func() {
		defer setEnvWithCleanup("UBER_OWNER", "")()
		ri := galileoRequestInterceptor{opts: &GalileoOpts{}}
		_, err := ri.Apply(context.TODO(), &transport.Request{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--disable-galileo")
		require.Contains(t, err.Error(), "Error getting your username")
	})
}

func TestYabDisabledMessageAuthenticateOut(t *testing.T) {
	setupWonka("user", func() {
		defer setEnvWithCleanup("UBER_OWNER", "user@uber.com")()
		ri := galileoRequestInterceptor{opts: &GalileoOpts{}}
		_, err := ri.Apply(context.TODO(), &transport.Request{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--disable-galileo")
		require.Contains(t, err.Error(), "Error authenticating")
	})
}

func TestOptsDisabled(t *testing.T) {
	opts := AddFlags()
	ri := NewRequestInterceptor(zap.L(), opts)
	opts.Disabled = true
	_, err := ri.Apply(context.TODO(), &transport.Request{})
	require.NoError(t, err)
}

func TestOptsClaims(t *testing.T) {
	setupWonka("user@uber.com", func() {
		claims := []string{"EVERYONE", "user@uber.com"}
		opts := AddFlags()
		opts.RequestClaims = strings.Join(claims, ",")
		ri := NewRequestInterceptor(zap.L(), opts)
		req, err := ri.Apply(context.TODO(), &transport.Request{TargetService: "foo"})
		require.NoError(t, err)

		claim, err := wonka.UnmarshalClaim(req.Baggage[contexthelper.ServiceAuthBaggageAttr])
		require.NoError(t, err)
		err = claim.Check("foo", claims)
		require.NoError(t, err)
	})
}

func TestGetUberOwnerEmailAddress(t *testing.T) {
	tests := []struct {
		email   string
		isValid bool
	}{
		{"user@atc.com", true},
		{"foober@uber.org", true},
		{"abc@ext.uber.com", true},
		{"atc.com", false},
		{"fuber@uber", false},
		{"fuber@", false},
		{"@uber.com", false},
		{"@.", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%+v", tt), func(t *testing.T) {
			defer setEnvWithCleanup("UBER_OWNER", tt.email)()
			_, ok := getUberOwner()
			if tt.isValid {
				require.True(t, ok, "getUberOwner should succeed on a valid email address")
			} else {
				require.False(t, ok, "getUberOwner should fail on an invalid email address")
			}
		})
	}
}
