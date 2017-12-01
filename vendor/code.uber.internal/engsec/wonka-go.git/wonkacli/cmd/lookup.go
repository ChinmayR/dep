package cmd

import (
	"github.com/urfave/cli"
	"go.uber.org/zap"
)

func performLookup(c CLIContext) error {
	log := zap.L()
	entity := c.StringOrFirstArg("entity")

	if entity == "" {
		log.Error("no entity name supplied to lookup")
		return cli.NewExitError("", 1)
	}

	log.Info("performing lookup", zap.Any("entity", entity))

	w, err := c.NewWonkaClient(DefaultClient)
	if err != nil {
		return cli.NewExitError("", 1)
	}

	result, err := w.Lookup(c.Context(), entity)
	if err != nil {
		log.Error("error looking up entity",
			zap.Any("request", entity),
			zap.Error(err),
		)

		return cli.NewExitError("", 1)
	}

	logWithEntity(*result, "entity found")
	return nil
}

// Lookup fetches information about a provided entity from Wonkamaster
func Lookup(c *cli.Context) error {
	return performLookup(cliWrapper{inner: c})
}
