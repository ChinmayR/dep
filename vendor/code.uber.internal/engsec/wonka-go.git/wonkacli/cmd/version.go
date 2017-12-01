package cmd

import (
	"fmt"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/urfave/cli"
)

// Version prints the wonka version and exits
func Version(c *cli.Context) error {
	fmt.Fprintf(c.App.Writer, "wonkacli and wonka library version %s\n", wonka.Version)
	fmt.Fprintf(c.App.Writer, "build version %s\n", wonka.BuildVersion)
	return nil
}
