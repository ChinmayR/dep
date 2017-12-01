package cmd

import (
	"encoding/base64"

	"code.uber.internal/engsec/wonka-go.git/internal/claimhelper"

	"github.com/urfave/cli"
	"go.uber.org/zap"
)

// Variables for testability
var _claimValidate = claimhelper.ClaimValidate
var _claimCheck = claimhelper.ClaimCheck

func performValidate(c CLIContext) error {
	log := zap.L()

	token := c.StringOrFirstArg("token")
	if token == "" {
		log.Error("no token provided to validate")
		return cli.NewExitError("", 1)
	}

	w, err := c.NewWonkaClient(DefaultClient)
	if err != nil {
		return cli.NewExitError("", 1)
	}

	if c.GlobalBool("json") {
		token = base64.StdEncoding.EncodeToString([]byte(token))
	}

	claimList := c.StringSlice("claim-list")

	log.Info("validating token")

	if len(claimList) == 0 {
		err = _claimValidate(token)
	} else {
		err = _claimCheck(claimList, w.EntityName(), token)
	}

	if err != nil {
		log.Error("token does not validate", zap.Error(err))
		return cli.NewExitError("", 1)
	}

	log.Info("token validates")
	return nil
}

// Validate ensures the claim is cryptographically correct.
func Validate(c *cli.Context) error {
	return performValidate(cliWrapper{inner: c})
}
