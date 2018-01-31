package cmd

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"testing"
	time "time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/keyhelper/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestEnrollWhenNoEntityShouldError(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Enroll(c)
	require.NotNil(t, err)
}

func TestEnrollWhenNoAllowedGroupsShouldSetDefault(t *testing.T) {
	ctx := context.Background()
	expectedArgToEnrollEntity := &wonka.Entity{
		EntityName:   "foo",
		PublicKey:    "fake-rsa-pub",
		ECCPublicKey: "fake-ecc-pub",
		Requires:     wonka.EveryEntity,
		Version:      wonka.SignEverythingVersion,
		SigType:      "SHA256",
	}

	k, err := rsa.GenerateKey(rand.Reader, 1024)
	require.Nil(t, err)

	keyHelper := new(mocks.KeyHelper)
	keyHelper.On("RSAAndECCFromFile", "fake-rsa-priv.pem").Return(k, "fake-rsa-pub", "fake-ecc-pub", nil)

	wonkaClient := new(MockWonka)
	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("String", "private-key").Return("fake-rsa-priv.pem")
	cliContext.On("StringSlice", "allowed-groups").Return([]string{})
	cliContext.On("Bool", "generate-keys").Return(false)
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewKeyHelper").Return(keyHelper, nil)

	n := int(time.Now().Unix())
	toSign := fmt.Sprintf("foo<%d>fake-rsa-pub", n)
	h := crypto.SHA256.New()
	h.Write([]byte(toSign))
	sig, err := k.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
	require.Nil(t, err)
	expectedArgToEnrollEntity.EntitySignature = base64.StdEncoding.EncodeToString(sig)
	expectedArgToEnrollEntity.Ctime = n

	// Match on everything except the Ctime since we want this test to be reliable.
	wonkaClient.On("EnrollEntity", ctx, mock.MatchedBy(func(arg interface{}) bool {
		a := arg.(*wonka.Entity)
		return a.EntityName == expectedArgToEnrollEntity.EntityName &&
			a.PublicKey == expectedArgToEnrollEntity.PublicKey &&
			a.ECCPublicKey == expectedArgToEnrollEntity.ECCPublicKey &&
			a.Requires == expectedArgToEnrollEntity.Requires &&
			a.Version == expectedArgToEnrollEntity.Version &&
			a.SigType == expectedArgToEnrollEntity.SigType &&
			a.EntitySignature == expectedArgToEnrollEntity.EntitySignature
	})).Return(expectedArgToEnrollEntity, nil)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err = performEnroll(cliContext)
	cliContext.AssertExpectations(t)
	require.Nil(t, err)
}

func TestEnrollWorksCorrectly(t *testing.T) {
	ctx := context.Background()
	expectedEntity := wonka.Entity{
		EntityName: "foo",
	}
	wonkaClient := new(MockWonka)
	wonkaClient.On("EnrollEntity", ctx, mock.AnythingOfType("*wonka.Entity")).Return(&expectedEntity, nil)

	keyHelper := new(mocks.KeyHelper)
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	require.Nil(t, err)
	keyHelper.On("RSAAndECCFromFile", "fake-rsa-priv.pem").Return(k, "fake-rsa-pub", "fake-ecc-pub", nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("String", "private-key").Return("fake-rsa-priv.pem")
	cliContext.On("StringSlice", "allowed-groups").Return([]string{"bar"})
	cliContext.On("Bool", "generate-keys").Return(false)
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewKeyHelper").Return(keyHelper, nil)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	err = performEnroll(cliContext)
	cliContext.AssertExpectations(t)
	require.Nil(t, err)
}

func TestEnrollWhenEnrollmentFailsShouldError(t *testing.T) {
	ctx := context.Background()
	wonkaClient := new(MockWonka)
	wonkaClient.On("EnrollEntity", ctx, mock.AnythingOfType("*wonka.Entity")).Return(nil, errors.New("fail"))

	keyHelper := new(mocks.KeyHelper)
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	require.Nil(t, err)
	keyHelper.On("RSAAndECCFromFile", "fake-rsa-priv.pem").Return(k, "fake-rsa-pub", "fake-ecc-pub", nil)

	cliContext := new(MockCLIContext)
	cliContext.On("GlobalString", "self").Return("bar")
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("String", "private-key").Return("fake-rsa-priv.pem")
	cliContext.On("StringSlice", "allowed-groups").Return([]string{})
	cliContext.On("Bool", "generate-keys").Return(false)
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewKeyHelper").Return(keyHelper, nil)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	err = performEnroll(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}
