package cmd

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"

	"github.com/urfave/cli"
	"go.uber.org/zap"
)

func resolveClaim(ctx context.Context, w wonka.Wonka, c *wonka.ClaimRequest) (*wonka.Claim, error) {
	log := zap.L()
	log.Info("resovling claim", zap.Any("entity", c.Destination))

	result, err := w.ClaimResolveTTL(ctx, c.Destination, time.Duration(c.Etime))
	if err != nil {
		log.Error("error resolving claim",
			zap.Any("request", c.Destination),
			zap.Error(err),
		)

		return nil, err
	}

	logWithClaim(*result, "resolve claim obtained")
	return result, nil
}

func requestClaim(ctx context.Context, w wonka.Wonka, c *wonka.ClaimRequest) (*wonka.Claim, error) {
	log := zap.L()
	log.Info("requesting claim",
		zap.Any("self", c.EntityName),
		zap.Any("destination", c.Destination),
		zap.Any("claim", c.Claim),
	)

	result, err := w.ClaimRequestTTL(ctx, c.Claim, c.Destination, time.Duration(c.Etime))
	if err != nil {
		log.Error("error requesting claim",
			zap.Any("request", c.Claim),
			zap.Any("scope", c.Destination),
			zap.Error(err),
		)

		return nil, err
	}

	logWithClaim(*result, "requested claim obtained")
	return result, nil
}

func impersonateClaim(ctx context.Context, w wonka.Wonka, c *wonka.ClaimRequest) (*wonka.Claim, error) {
	log := zap.L()
	log.Info("impersonating claim",
		zap.Any("source", c.EntityName),
		zap.Any("destination", c.Destination),
		zap.Any("claim", c.Claim),
	)

	result, err := w.ClaimImpersonateTTL(ctx, c.ImpersonatedEntity, c.Destination, time.Duration(c.Etime))
	if err != nil {
		log.Error("error impersonating claim", zap.Error(err))
		return nil, err
	}

	logWithClaim(*result, "impersonated claim obtained")
	log.With(
		zap.Any("claim_name", result.EntityName),
		zap.Any("impersonated_user", c.ImpersonatedEntity),
		zap.Any("impersonator", c.EntityName),
		zap.Any("claim", strings.Join(result.Claims, ",")),
		zap.Any("destination", result.Destination),
		zap.Any("t", result.ValidAfter),
		zap.Any("validAfter", time.Unix(result.ValidAfter, 0)),
		zap.Any("validBefore", time.Unix(result.ValidBefore, 0)),
		zap.Any("validFor", time.Unix(result.ValidBefore, 0).Sub(time.Unix(result.ValidAfter, 0))),
		zap.Any("signature", base64.StdEncoding.EncodeToString(result.Signature)),
	).Info("impersonated claim obtained")
	return result, nil
}

func performRequest(c CLIContext) error {
	log := zap.L()
	claim := c.StringOrFirstArg("claim")
	source := c.String("source")
	destination := c.String("destination")
	expiration := c.Duration("expiration")
	self := c.GlobalString("self")
	outputPath := c.String("output")

	if claim == "" && destination == "" {
		log.Error("must supply claim or destination to request")
		return cli.NewExitError("", 1)
	}

	if destination == "" {
		destination = self
	}

	// note that we abuse EntityName to be self
	request := &wonka.ClaimRequest{
		EntityName:         self,
		Claim:              claim,
		ImpersonatedEntity: source,
		Destination:        destination,
		Etime:              int64(expiration),
	}

	var w wonka.Wonka
	var err error
	var result *wonka.Claim

	if source == "" { // this is either a resolve or request
		w, err = c.NewWonkaClient(DefaultClient)
		if err != nil {
			return cli.NewExitError("", 1)
		}

		if claim == "" { // this is a resolve (since only --destination was provided)
			result, err = resolveClaim(c.Context(), w, request)
		} else { // this is a request
			result, err = requestClaim(c.Context(), w, request)
		}
	} else { // this is an impersonation
		w, err = c.NewWonkaClient(ImpersonationClient)
		if err != nil {
			return cli.NewExitError("", 1)
		}
		// make the request with the authorized client
		result, err = impersonateClaim(c.Context(), w, request)
	}

	if err != nil {
		log.Error("error obtaining claim", zap.Error(err))
		return cli.NewExitError("", 1)
	}

	var output string
	if c.GlobalBool("json") {
		output, err = marshalClaimJSON(result)
	} else {
		output, err = wonka.MarshalClaim(result)
	}

	if err != nil {
		log.Error("error converting claim to desired format",
			zap.Error(err),
		)
		return cli.NewExitError("", 1)
	}

	// finally, let's write the output in the method desired
	if outputPath != "" {
		err = ioutil.WriteFile(outputPath, []byte(output), 0600)
		if err != nil {
			log.Error("error writing claim to file", zap.Error(err))
			return cli.NewExitError("", 1)
		}
	} else {
		writer := c.Writer()
		if writer != nil {
			writer.Write([]byte(output))
			writer.Write([]byte("\n"))
		}
	}

	return nil
}

// Request a claim for the provided entity
// Note that this may perform three different operations in Wonka:
// 1) Request a claim if a claim is provided
// 2) Resolve a claim if --destination is provided, but a claim is not
// 3) Impersonate a request (if the source is specified, we attempt to impersonate)
func Request(c *cli.Context) error {
	return performRequest(cliWrapper{inner: c})
}
