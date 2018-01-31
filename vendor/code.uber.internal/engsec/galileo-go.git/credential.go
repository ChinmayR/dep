package galileo

import (
	"context"
	"fmt"
)

// CredentialGalileo extends Galileo interface with methods that operate on
// tokens as strings.
type CredentialGalileo interface {
	Galileo

	GetCredential(ctx context.Context, opts ...CredentialConfigurationOption) (string, error)
	ValidateCredential(token string, opts ...CredentialValidationOption) error
}

var _ CredentialGalileo = (*uberGalileo)(nil)

// GetCredential returns a marshalled Wonka token
func GetCredential(ctx context.Context, g Galileo, opts ...CredentialConfigurationOption) (string, error) {
	cg, ok := g.(CredentialGalileo)
	if !ok {
		return "", fmt.Errorf("g is type %T and not a CredentialGalileo", g)
	}

	return cg.GetCredential(ctx, opts...)
}

// ValidateCredential returns nil if the token is valid, and the validation
// error otherwise.
func ValidateCredential(g Galileo, token string, opts ...CredentialValidationOption) error {
	cg, ok := g.(CredentialGalileo)
	if !ok {
		return fmt.Errorf("g is type %T and not a CredentialGalileo", g)
	}

	return cg.ValidateCredential(token, opts...)
}
