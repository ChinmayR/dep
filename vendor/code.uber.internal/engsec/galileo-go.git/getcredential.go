package galileo

import (
	"context"
	"errors"
	"fmt"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

// CredentialConfigurationOption allows users to specify details about
// credentials they want.
type CredentialConfigurationOption interface {
	applyCredentialConfigurationOption(*credentialConfiguration)
}

// credentialConfiguration stores details about credentials that can be
// obtained. In future we may support multiple types of credentials, or multiple
// credential authorities.
type credentialConfiguration struct {
	Destination    string
	ExplicitClaims string
}

// destinationOption sets the Destination field of a Wonka token.
type destinationOption string

// applyCredentialValidationOption implements CredentialConfigurationOption
func (d destinationOption) applyCredentialConfigurationOption(cfg *credentialConfiguration) {
	cfg.Destination = string(d)
}

// WithDestinationService specifies the credential should be valid for the given
// service.
func WithDestinationService(d string) CredentialConfigurationOption {
	return destinationOption(d)
}

// GetCredential returns a marshalled Wonka token. The given context is used for
// remote call to obtain Wonka token, if required, and is not modified.
func (u *uberGalileo) GetCredential(ctx context.Context, opts ...CredentialConfigurationOption) (string, error) {
	cfg := &credentialConfiguration{}
	for _, opt := range opts {
		opt.applyCredentialConfigurationOption(cfg)
	}

	if cfg.Destination == "" {
		return "", errors.New("WithDestinationService option is required")
	}

	if u.isDisabled() || u.shouldSkipDest(cfg.Destination) {
		return "", nil
	}

	return u.doGetCredential(ctx, cfg.Destination, cfg.ExplicitClaims)
}

// doGetCredential returns a marshalled wonka token, potentially from the
// outbound cache. The re-usable bits between GetCredential and
// doAuthenicateOut.
func (u *uberGalileo) doGetCredential(ctx context.Context, destination string, explicitClaim string) (string, error) {
	claim, err := u.resolveClaimWithCache(ctx, destination, explicitClaim)
	if err != nil {
		return "", err
	}

	// Stamp this resolved authentication claim data into the context/ctx flow
	token, err := wonka.MarshalClaim(claim)
	if err != nil {
		return "", fmt.Errorf("error marshalling claim: %v", err)
	}

	return token, nil
}
