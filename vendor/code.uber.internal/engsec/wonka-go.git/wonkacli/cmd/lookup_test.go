package cmd

import (
	"context"
	"errors"
	"flag"
	"testing"

	"code.uber.internal/engsec/wonka-go.git"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestLookupWhenNoEntityShouldError(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Lookup(c)
	require.NotNil(t, err)
}

func TestLookupWhenCannotCreateWonkaClientShouldFail(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("NewWonkaClient", DefaultClient).Return(nil, errors.New("fail"))
	err := performLookup(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestLookupWorksCorrectly(t *testing.T) {
	ctx := context.Background()
	wonkaClient := new(MockWonka)
	entity := wonka.Entity{EntityName: "foo"}
	wonkaClient.On("Lookup", ctx, "foo").Return(&entity, nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performLookup(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.Nil(t, err)
}

func TestLookupWhenWonkaLookupFailsShouldError(t *testing.T) {
	ctx := context.Background()
	wonkaClient := new(MockWonka)
	wonkaClient.On("Lookup", ctx, "foo").Return(nil, errors.New("fail"))

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performLookup(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	require.NotNil(t, err)
}
