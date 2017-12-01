package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoSelfFails(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoTaskIDFails(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoKeyPathFails(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("foo-1234")
	cliCtx.On("String", "key-path").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoCertPathFails(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("foo-1234")
	cliCtx.On("String", "key-path").Return("foo.key")
	cliCtx.On("String", "cert-path").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}

func TestNoWonkadPathFails(t *testing.T) {
	cliCtx := new(MockCLIContext)
	cliCtx.On("GlobalString", "self").Return("foo")
	cliCtx.On("String", "taskid").Return("foo-1234")
	cliCtx.On("String", "key-path").Return("foo.key")
	cliCtx.On("String", "cert-path").Return("foo.crt")
	cliCtx.On("String", "wonkad-path").Return("")
	err := doRequestCertificate(cliCtx)
	require.Error(t, err, "should error")
}
