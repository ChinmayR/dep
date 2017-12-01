package cmd

import (
	"code.uber.internal/engsec/wonka-go.git"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func performDelete(c CLIContext) error {
	entity := c.StringOrFirstArg("entity")

	if entity == "" {
		logrus.Error("no entity name supplied to delete")
		return cli.NewExitError("", 1)
	}
	logrus.WithField("entity", entity).Info("deleting entity")

	adminReq := wonka.AdminRequest{
		Action:     wonka.DeleteEntity,
		ActionOn:   entity,
		EntityName: c.GlobalString("self"),
	}

	w, err := c.NewWonkaClient(DeletionClient)
	if err != nil {
		return cli.NewExitError("", 1)
	}

	if err := w.Admin(c.Context(), adminReq); err != nil {
		logrus.WithField("error", err).Error("error deleting entity")
		return cli.NewExitError("", 1)
	}

	logrus.WithField("entity", entity).Info("entity successfully deleted")
	return nil
}

// Delete an entity
func Delete(c *cli.Context) error {
	return performDelete(cliWrapper{inner: c})
}
