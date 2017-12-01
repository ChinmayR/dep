package cmd

import (
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestValidateWhenNoTokenShouldError(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Validate(c)
	require.NotNil(t, err)
}

func TestValidateWhenCannotCreateWonkaClientShouldError(t *testing.T) {
	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "token").Return("foo")
	cliContext.On("NewWonkaClient", DefaultClient).Return(nil, errors.New("fail"))
	err := performValidate(cliContext)
	cliContext.AssertExpectations(t)
	require.NotNil(t, err)
}

func TestValidateWorksCorrectlyWithoutClaimList(t *testing.T) {
	token := "foo"
	var actualToken string

	oldValidate := _claimValidate
	defer func() { _claimValidate = oldValidate }()

	_claimValidate = func(t string) error {
		actualToken = t
		return nil
	}

	wonkaClient := new(MockWonka)
	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "token").Return(token)
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("StringSlice", "claim-list").Return([]string{})
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performValidate(cliContext)
	cliContext.AssertExpectations(t)
	assert.Equal(t, token, actualToken, "claim validate was called incorrectly")
	require.Nil(t, err)
}

func TestValidateWorksCorrectlyWithClaimList(t *testing.T) {
	entity := "hilda"
	token := "jrr"
	claims := []string{"mussels", "oysters"}
	called := false

	oldCheck := _claimCheck
	defer func() { _claimCheck = oldCheck }()

	_claimCheck = func(actualClaims []string, actualDestination, actualToken string) error {
		called = true
		assert.Equal(t, claims, actualClaims, "checking unexpected claims")
		assert.Equal(t, entity, actualDestination, "checking unexpected destination")
		assert.Equal(t, token, actualToken, "checking unexpected token")
		return nil
	}

	wonkaClient := new(MockWonka)
	wonkaClient.On("EntityName").Return(entity) // entity is typically passed as --self

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "token").Return(token)
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("StringSlice", "claim-list").Return(claims)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performValidate(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	assert.True(t, called, "claim check function was not called")
	require.Nil(t, err)
}

func TestValidateWhenClaimValidateFailsShouldError(t *testing.T) {
	token := "foo"
	var actualToken string

	oldValidate := _claimValidate
	defer func() { _claimValidate = oldValidate }()

	_claimValidate = func(t string) error {
		actualToken = t
		return errors.New("validate failed")
	}

	wonkaClient := new(MockWonka)

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "token").Return(token)
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("StringSlice", "claim-list").Return([]string{})
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performValidate(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	assert.Equal(t, token, actualToken, "claim validate was called incorrectly")
	require.Error(t, err)
}

func TestValidateWhenClaimCheckFailsShouldError(t *testing.T) {
	entity := "hilda"
	token := "jrr"
	claims := []string{"mussels", "oysters"}
	called := false

	oldCheck := _claimCheck
	defer func() { _claimCheck = oldCheck }()

	_claimCheck = func(actualClaims []string, actualDestination, actualToken string) error {
		called = true
		assert.Equal(t, claims, actualClaims, "checking unexpected claims")
		assert.Equal(t, entity, actualDestination, "checking unexpected destination")
		assert.Equal(t, token, actualToken, "checking unexpected token")
		return errors.New("invalid token")
	}

	wonkaClient := new(MockWonka)
	wonkaClient.On("EntityName").Return(entity) // entity is typically passed as --self

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "token").Return(token)
	cliContext.On("GlobalBool", "json").Return(false)
	cliContext.On("StringSlice", "claim-list").Return(claims)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performValidate(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	assert.True(t, called, "claim check function was not called")
	require.Error(t, err)
}

func TestValidateWhenJSONShouldEncodeToken(t *testing.T) {
	entity := "barney"
	inputToken := "foo"
	expectedToken := "Zm9v" // base64 of foo
	claims := []string{"mussels", "oysters"}
	called := false

	oldCheck := _claimCheck
	defer func() { _claimCheck = oldCheck }()

	_claimCheck = func(actualClaims []string, actualDestination, actualToken string) error {
		called = true
		assert.Equal(t, claims, actualClaims, "checking unexpected claims")
		assert.Equal(t, entity, actualDestination, "checking unexpected destination")
		assert.Equal(t, expectedToken, actualToken, "checking unexpected token")
		return nil
	}

	wonkaClient := new(MockWonka)
	wonkaClient.On("EntityName").Return(entity) // entity is typically passed as --self

	cliContext := new(MockCLIContext)
	cliContext.On("StringOrFirstArg", "token").Return(inputToken)
	cliContext.On("GlobalBool", "json").Return(true)
	cliContext.On("StringSlice", "claim-list").Return(claims)
	cliContext.On("NewWonkaClient", DefaultClient).Return(wonkaClient, nil)

	err := performValidate(cliContext)
	cliContext.AssertExpectations(t)
	wonkaClient.AssertExpectations(t)
	assert.True(t, called, "claim check function was not called")
	require.NoError(t, err)
}
