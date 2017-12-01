package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"path"
	"testing"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkatestdata"

	"github.com/stretchr/testify/require"
)

func TestLoadSignData(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	b, err := loadDataToSign(cliCtx)
	require.NoError(t, err, "shouldn't error: %v", err)
	require.Equal(t, b, []byte("foober"))
}

func TestLoadSignFile(t *testing.T) {
	wonkatestdata.WithTempDir(func(dir string) {
		f := path.Join(dir, "file")
		err := ioutil.WriteFile(f, []byte("foober"), 0444)
		require.NoError(t, err, "error writing file: %v", err)

		cliCtx := new(MockCLIContext)
		cliCtx.On("StringOrFirstArg", "data").Return("")
		cliCtx.On("StringOrFirstArg", "file").Return(f)
		b, err := loadDataToSign(cliCtx)
		require.NoError(t, err, "shouldn't error: %v", err)
		require.Equal(t, b, []byte("foober"))
	})
}

func TestLoadSignDataError(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("")
	cliCtx.On("StringOrFirstArg", "file").Return("")
	b, err := loadDataToSign(cliCtx)
	require.Error(t, err, "should error")
	require.Nil(t, b, "should be empty")
}

func TestSignNoDataError(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("")
	cliCtx.On("StringOrFirstArg", "file").Return("")

	err := doSignData(cliCtx)
	require.Error(t, err, "sign should err")
}

func TestSignDataError(t *testing.T) {
	wonkaClient := new(MockWonka)
	wonkaClient.On("Sign", []byte("foober")).Return(nil, errors.New("err"))

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doSignData(cliCtx)
	require.Error(t, err, "signing should error")
}

func TestSignData(t *testing.T) {
	wonkaClient := new(MockWonka)
	wonkaClient.On("Sign", []byte("foober")).Return([]byte("test"), nil)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)
	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doSignData(cliCtx)
	require.NoError(t, err, "signing shouldn't err: %v", err)
}

func TestVerifySucceeds(t *testing.T) {
	ctx := context.Background()
	signString := "foober"

	wonkaClient := new(MockWonka)
	wonkaClient.On("Verify", ctx, []byte("foober"), []byte("foober"), "foober").Return(true)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString([]byte(signString)))

	cliCtx.On("Context").Return(ctx)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)

	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doVerifySignature(cliCtx)
	require.NoError(t, err, "verify shouldn't err: %v", err)
}

func TestVerifyFails(t *testing.T) {
	ctx := context.Background()
	signString := "foober"

	wonkaClient := new(MockWonka)
	wonkaClient.On("Verify", ctx, []byte("foober"), []byte("foober"), "foober").Return(false)

	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return(signString)
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").
		Return(base64.StdEncoding.EncodeToString([]byte(signString)))

	cliCtx.On("Context").Return(ctx)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)

	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doVerifySignature(cliCtx)
	require.Error(t, err, "verify should err")
}

func TestVerifyBadSignature(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("foober")
	cliCtx.On("StringOrFirstArg", "signer").Return("foober")
	cliCtx.On("StringOrFirstArg", "signature").Return("foober")

	wonkaClient := new(MockWonka)
	cliCtx.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliCtx.On("Bool", "cert").Return(false)

	cliCtx.On("Writer").Return(ioutil.Discard)
	err := doVerifySignature(cliCtx)
	require.Error(t, err, "verify should err")
}

func TestVerifyNoDataError(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("StringOrFirstArg", "data").Return("")
	cliCtx.On("StringOrFirstArg", "file").Return("")

	err := doVerifySignature(cliCtx)
	require.Error(t, err, "verify should err")
}
