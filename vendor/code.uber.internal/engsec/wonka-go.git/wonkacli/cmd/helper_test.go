package cmd

import (
	"context"
	"flag"
	"path"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/testdata"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"github.com/urfave/cli"
)

// Configure a cliWrapper with a context.Context in app metadata so we don't
// segfault in tests when NewWonkaClientFromConfig calls wonka.InitWithContext
func newCliWrapper() cliWrapper {
	a := cli.NewApp()
	a.Setup()
	a.Metadata["ctx"] = context.Background()
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(a, fs, nil)
	return cliWrapper{inner: c}
}

func TestNewWonkaClientFromConfigWhenConfigIsEmptyThenShouldError(t *testing.T) {
	c := newCliWrapper()
	wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
		w, err := c.NewWonkaClientFromConfig(wonka.Config{})
		require.Nil(t, w)
		require.Error(t, err)
	})
}

func TestNewWonkaClientFromConfigWorksCorrectly(t *testing.T) {
	c := newCliWrapper()

	testdata.WithTempDir(func(dir string) {
		k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := testdata.WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			pubKey := testdata.PublicPemFromKey(testdata.PrivateKeyFromPem(testdata.RSAPrivKey))
			e := wonka.Entity{
				EntityName:   "helper-test",
				ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
				PublicKey:    string(pubKey),
			}
			err := handlerCfg.DB.Create(context.TODO(), &e)
			require.NoError(t, err, "create entity")

			cfg := wonka.Config{
				EntityName:     "helper-test",
				PrivateKeyPath: privKeyPath,
			}

			w, err := c.NewWonkaClientFromConfig(cfg)
			require.NotNil(t, w)
			require.NoError(t, err)
		})
	})
}

/*
func TestNewClaimBaggageWhenPermissionDeniedShouldError(t *testing.T) {
	testdata.WithTempDir(func(dir string) {
		k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := testdata.WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)

			pubKey := testdata.PublicPemFromKey(testdata.PrivateKeyFromPem(testdata.RSAPrivKey))
			e := wonka.Entity{
				EntityName:   "helper-test",
				ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
				PublicKey:    string(pubKey),
			}
			ok := handlerCfg.DB.CreateEntity(e)
			require.True(t, ok, "create entity")

			cfg := wonka.Config{
				EntityName:     "helper-test",
				PrivateKeyPath: privKeyPath,
				Metrics:        tally.NoopScope,
			}

			pubKeyPath := path.Join(dir, "wonka_public.pem")
			testdata.WritePublicKey(&k.PublicKey, pubKeyPath)

			fs := flag.NewFlagSet(
				"testing",
				flag.ContinueOnError,
			)
			app := cli.App{
				Metadata: map[string]interface{}{"config": cfg},
			}
			cliCtx := cli.NewContext(&app, fs, nil)
			c := cliWrapper{inner: cliCtx}

			ctx, err := c.NewClaimBaggage(EnrollmentClient)
			require.Nil(t, ctx)
			require.Error(t, err)

			ctx, err = c.NewClaimBaggage(DeletionClient)
			require.Nil(t, ctx)
			require.Error(t, err)

			ctx, err = c.NewClaimBaggage(ImpersonationClient)
			require.Nil(t, ctx)
			require.Error(t, err)
		})
	})
}

func TestNewClaimBaggageWorksCorrectly(t *testing.T) {
	testdata.WithTempDir(func(dir string) {
		k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := testdata.WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)
			require.NotNil(t, r, "setuphandlers returned nil")

			pubKey := testdata.PublicPemFromKey(testdata.PrivateKeyFromPem(testdata.RSAPrivKey))
			e := wonka.Entity{
				EntityName:   "helper-test",
				ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
				PublicKey:    string(pubKey),
			}
			ok := handlerCfg.DB.CreateEntity(e)
			require.True(t, ok, "create entity")

			cfg := wonka.Config{
				EntityName:     "helper-test",
				PrivateKeyPath: privKeyPath,
				Metrics:        tally.NoopScope,
			}

			pubKeyPath := path.Join(dir, "wonka_public.pem")
			testdata.WritePublicKey(&k.PublicKey, pubKeyPath)

			fs := flag.NewFlagSet(
				"testing",
				flag.ContinueOnError,
			)
			fs.String("public-key", pubKeyPath, "")
			fs.String("private-key", privKeyPath, "")
			fs.String("self", privKeyPath, "helper-test")
			fs.Parse([]string{})
			app := cli.App{
				Metadata: map[string]interface{}{"config": cfg},
			}
			cliCtx := cli.NewContext(&app, fs, nil)
			c := cliWrapper{inner: cliCtx}

			ctx, err := c.NewClaimBaggage(EnrollmentClient)
			require.Nil(t, ctx)
			require.Error(t, err)

			ctx, err = c.NewClaimBaggage(DeletionClient)
			require.Nil(t, ctx)
			require.Error(t, err)

			ctx, err = c.NewClaimBaggage(ImpersonationClient)
			require.NotNil(t, ctx)
			require.NoError(t, err)
		})
	})
}
*/

func TestNewWonkaClientWorksCorrectly(t *testing.T) {
	testdata.WithTempDir(func(dir string) {
		k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := testdata.WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		wonkatestdata.WithWonkaMaster("", func(r common.Router, handlerCfg common.HandlerConfig) {
			handlers.SetupHandlers(r, handlerCfg)
			require.NotNil(t, r, "setuphandlers returned nil")

			pubKey := testdata.PublicPemFromKey(testdata.PrivateKeyFromPem(testdata.RSAPrivKey))
			e := wonka.Entity{
				EntityName:   "helper-test",
				ECCPublicKey: wonkatestdata.ECCPublicFromPrivateKey(k),
				PublicKey:    string(pubKey),
			}
			err := handlerCfg.DB.Create(context.TODO(), &e)
			require.NoError(t, err, "create entity")

			cfg := wonka.Config{
				EntityName:     "helper-test",
				PrivateKeyPath: privKeyPath,
				Metrics:        tally.NoopScope,
			}

			fs := flag.NewFlagSet(
				"testing",
				flag.ContinueOnError,
			)

			fs.String("private-key", privKeyPath, "")
			fs.Parse([]string{""})
			app := cli.App{
				Metadata: map[string]interface{}{
					"config": cfg,
					"ctx":    context.Background(),
				},
			}
			cliCtx := cli.NewContext(&app, fs, nil)
			c := cliWrapper{inner: cliCtx}

			ctx, err := c.NewWonkaClient(EnrollmentClient)
			require.Nil(t, ctx)
			require.Error(t, err)

			ctx, err = c.NewWonkaClient(ImpersonationClient)
			require.Nil(t, ctx)
			require.Error(t, err)
		})
	})
}
