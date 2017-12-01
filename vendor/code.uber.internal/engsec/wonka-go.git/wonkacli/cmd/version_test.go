package cmd

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestVersion(t *testing.T) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	c := cli.NewContext(cli.NewApp(), fs, nil)
	err := Version(c)
	require.Nil(t, err)
}
