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

func TestDeleteWhenNoEntityShouldError(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Delete(c)
	require.NotNil(t, err)
}

func TestDeleteWhenCannotCreateWonkaClientShouldError(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("GlobalString", "self").Return("bar")
	cliContext.On("NewWonkaClient", DeletionClient).Return(nil, errors.New("fail"))
	err := performDelete(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDeleteWhenCannotDeleteShouldError(t *testing.T) {
	ctx := context.Background()
	expectedReq := wonka.AdminRequest{
		Action:     wonka.DeleteEntity,
		ActionOn:   "foo",
		EntityName: "bar",
	}
	wonkaClient := new(MockWonka)
	wonkaClient.On("Admin", ctx, expectedReq).Return(errors.New("fail"))

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("GlobalString", "self").Return("bar")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DeletionClient).Return(wonkaClient, nil)
	err := performDelete(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestDeleteWhenWorksCorrectly(t *testing.T) {
	ctx := context.Background()
	expectedReq := wonka.AdminRequest{
		Action:     wonka.DeleteEntity,
		ActionOn:   "foo",
		EntityName: "bar",
	}
	wonkaClient := new(MockWonka)
	wonkaClient.On("Admin", ctx, expectedReq).Return(nil)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "entity").Return("foo")
	cliContext.On("GlobalString", "self").Return("bar")
	cliContext.On("Context").Return(ctx)
	cliContext.On("NewWonkaClient", DeletionClient).Return(wonkaClient, nil)
	err := performDelete(cliContext)
	cliContext.AssertExpectations(t)
	require.Nil(t, err)
}
