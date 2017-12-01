package cmd

import (
	"context"
	"errors"
	"flag"
	"io/ioutil"
	"testing"
	"time"

	"code.uber.internal/engsec/wonka-go.git"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestRequestWhenNoClaimOrDestinationThenShouldError(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Request(c)
	require.NotNil(t, err)
}

func TestRequestWhenNoSourceAndNoClaimThenShouldResolve(t *testing.T) {
	ctx := context.Background()
	claim := &wonka.Claim{}

	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimResolveTTL", ctx, "foo", time.Minute).Return(claim, nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("")
	cliContext.On("String", "source").Return("")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("Context").Return(ctx)
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliContext.On("Writer").Return(ioutil.Discard)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.Nil(t, err)
}

func TestRequestWhenNoSourceButClaimThenShouldRequest(t *testing.T) {
	ctx := context.Background()
	claim := &wonka.Claim{}

	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimRequestTTL", ctx, "foo", "foo", time.Minute).Return(claim, nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("foo")
	cliContext.On("String", "source").Return("")
	cliContext.On("String", "destination").Return("")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliContext.On("Writer").Return(ioutil.Discard)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.Nil(t, err)
}

func TestRequestWhenRequestErrorsThenShouldError(t *testing.T) {
	ctx := context.Background()
	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimRequestTTL", ctx, "foo", "foo", time.Minute).Return(nil, errors.New("fail"))

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("foo")
	cliContext.On("String", "source").Return("")
	cliContext.On("String", "destination").Return("")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestRequestWhenSourceThenShouldImpersonate(t *testing.T) {
	ctx := context.Background()
	claim := &wonka.Claim{}

	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimImpersonateTTL", ctx, "foo", "foo", time.Minute).Return(claim, nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("foo")
	cliContext.On("String", "source").Return("foo")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", ImpersonationClient).Return(wonkaClient, nil)
	cliContext.On("Writer").Return(ioutil.Discard)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.Nil(t, err)
}

func TestRequestWhenImpersonationFailsThenShouldError(t *testing.T) {
	ctx := context.Background()
	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimImpersonateTTL", ctx, "foo", "foo", time.Minute).Return(nil, errors.New("fail"))

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("foo")
	cliContext.On("String", "source").Return("foo")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", ImpersonationClient).Return(wonkaClient, nil)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestRequestWhenCreatingImpersonationClientFailsThenShouldError(t *testing.T) {
	wonkaClient := new(MockWonka)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("foo")
	cliContext.On("String", "source").Return("foo")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("NewWonkaClient", ImpersonationClient).Return(nil, errors.New("fail"))

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestRequestWhenErrorGettingClaimThenShouldError(t *testing.T) {
	ctx := context.Background()
	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimResolveTTL", ctx, "foo", time.Minute).Return(nil, errors.New("fail"))

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("")
	cliContext.On("String", "source").Return("")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestRequestWhenErrorCreatingWonkaClientThenShouldError(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("")
	cliContext.On("String", "source").Return("")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("NewWonkaClient", DefaultClient).Return(nil, errors.New("fail"))

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestRequestWhenJSONSpecifiedThenShouldJSONify(t *testing.T) {
	ctx := context.Background()
	claim := &wonka.Claim{}

	wonkaClient := new(MockWonka)
	wonkaClient.On("ClaimResolveTTL", ctx, "foo", time.Minute).Return(claim, nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "claim").Return("")
	cliContext.On("String", "source").Return("")
	cliContext.On("String", "destination").Return("foo")
	cliContext.On("Duration", "expiration").Return(time.Minute)
	cliContext.On("GlobalString", "self").Return("foo")
	cliContext.On("String", "output").Return("")
	cliContext.On("GlobalBool", "json").Return(true)
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)
	cliContext.On("Writer").Return(ioutil.Discard)

	err := performRequest(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.Nil(t, err)
}
