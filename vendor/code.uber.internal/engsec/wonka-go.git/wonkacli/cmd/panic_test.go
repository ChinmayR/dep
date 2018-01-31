package cmd

import (
	"flag"
	"io/ioutil"
	"path"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git/testdata"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestDisableWhenNoPrivateKeyShouldError(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := SignDisableMessage(c)
	require.NotNil(t, err)
}

func TestDisableWorksCorrectly(t *testing.T) {
	testdata.WithTempDir(func(dir string) {
		k := testdata.PrivateKeyFromPem(testdata.RSAPrivKey)
		privKeyPath := path.Join(dir, "wonka_private.pem")
		e := testdata.WritePrivateKey(k, privKeyPath)
		require.NoError(t, e, "writing privkey")

		pubKeyPath := path.Join(dir, "wonka_public.pem")
		e = testdata.WritePublicKey(&k.PublicKey, pubKeyPath)
		require.NoError(t, e, "writing pubkey")

		cliContext := new(MockCLIContext)
		cliContext.On("GlobalBool", "json").Return(false)
		cliContext.On("GlobalString", "self").Return("bar")
		cliContext.On("Duration", "expiration").Return(time.Minute)
		cliContext.On("GlobalString", "public-key").Return(pubKeyPath)
		cliContext.On("GlobalString", "private-key").Return(privKeyPath)
		cliContext.On("Writer").Return(ioutil.Discard)
		result := performSignDisableMessage(cliContext)
		require.Nil(t, result)
	})
}

func TestDisableWithNegativeExpirationError(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("Duration", "expiration").Return(-1 * time.Minute)
	err := performSignDisableMessage(cliContext)
	require.NotNil(t, err)
}
