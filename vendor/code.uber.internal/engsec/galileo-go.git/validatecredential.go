package galileo

import (
	"strings"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/claimtools"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	wonka "code.uber.internal/engsec/wonka-go.git"
)

// CredentialValidationOption allows users to specify requirements for a valid
// credential and configure how a credential should be validated.
type CredentialValidationOption interface {
	applyCredentialValidationOption(*validationConfiguration)
}

// validationConfiguration stores details about how a credential should be
// validated. In future we may support multiple types of credentials, or
// multiple credential authorities.
type validationConfiguration struct {
	AllowedDestinations []string
	AllowedEntities     []string
	CallerName          string
}

// allowedEntitiesOption sets the allowed values for the e field of a Wonka
// token.
type allowedEntitiesOption []string

// applyCredentialValidationOption implements CredentialValidationOption
func (a allowedEntitiesOption) applyCredentialValidationOption(cfg *validationConfiguration) {
	cfg.AllowedEntities = append(cfg.AllowedEntities, []string(a)...)
}

// AllowedEntities is the list of entity names to from which a token is valid.
func AllowedEntities(entities ...string) CredentialValidationOption {
	return allowedEntitiesOption(entities)
}

// callerName allows setting CallerName credential validation.
type callerNameOption string

// applyCredentialValidationOption implements CredentialValidationOption
func (cn callerNameOption) applyCredentialValidationOption(cfg *validationConfiguration) {
	cfg.CallerName = string(cn)
}

// CallerName tells credential validator the name claimed by the remote caller
// making the current inbound request. This value typically comes from an
// unauthenticated rpc header, e.g. x-uber-source or rpc-caller. Validator may
// choose to allow unauthenticated requests from certain callers, e.g. members
// of Wonkamaster's derelict services list.
// Has no effect on validation when provided name is empty string.
func CallerName(cn string) CredentialValidationOption {
	return callerNameOption(cn)
}

// ValidateCredential returns nil if the token is valid, and the validation
// error otherwise.
func (u *uberGalileo) ValidateCredential(token string, opts ...CredentialValidationOption) error {
	if u.isDisabled() {
		return nil
	}

	cfg := validationConfiguration{
		AllowedDestinations: u.serviceAliases,
	}

	for _, opt := range opts {
		opt.applyCredentialValidationOption(&cfg)
	}

	if len(cfg.AllowedEntities) == 0 {
		cfg.AllowedEntities = u.allowedEntities
	}

	if u.IsDerelict(cfg.CallerName) {
		return nil
	}

	claim, err := contexthelper.UnmarshalToken(token)
	if err != nil {
		return err
	}

	// TODO(tjulian): add actual claim caching here
	return validateClaim(claimtools.NewCachedClaimDisabled(claim), cfg)
}

// validateClaim is the common parts between doAuthenticateIn and
// ValidateCredential.
func validateClaim(cacheableClaim *claimtools.CacheableClaim, cfg validationConfiguration) internal.InboundAuthenticationError {
	if len(cfg.AllowedEntities) == 0 {
		// When service owner did not configure any allowed entities, the
		// default behavior is to allow all entities.
		cfg.AllowedEntities = cacheableClaim.Claims
	} else {
		// If we accept wonka.EveryEntity, add the entity name on the remote claim.
		// This is just in case the remote side got an identity claim for their
		// entity name rather than one for wonka.EveryEntity.
		for _, c := range cfg.AllowedEntities {
			if strings.EqualFold(c, wonka.EveryEntity) {
				cfg.AllowedEntities = append(cfg.AllowedEntities, cacheableClaim.EntityName)
				break
			}
		}
	}

	if checkErr := cacheableClaim.Inspect(cfg.AllowedDestinations, cfg.AllowedEntities); checkErr != nil {
		reason := internal.UnauthorizedInvalidToken
		if r, ok := wonka.GetInvalidityReason(checkErr); ok {
			reason = internal.UnauthorizedReason(r)
		}
		return internal.NewInboundAuthenticationErrorf(
			reason, true, /* has baggage */
			"not permitted by configuration: %v", checkErr,
		)
	}

	return nil
}
