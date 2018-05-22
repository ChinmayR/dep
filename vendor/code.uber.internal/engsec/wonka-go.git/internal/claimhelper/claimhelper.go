package claimhelper

import wonka "code.uber.internal/engsec/wonka-go.git"

// Encapsulating the token unmarshal with check or validate is very useful for
// testing. And, we want to keep these out of the public API.
// Eventually we'll have a ClaimCheckerService, or similar, to replace these.

// ClaimCheck unmarshals and validates a claim token.
// See Claim.Check for details.
func ClaimCheck(requiredClaims []string, dest, claimToken string) error {
	claim, err := wonka.UnmarshalClaim(claimToken)
	if err != nil {
		return err
	}
	return claim.Check(dest, requiredClaims)
}

// ClaimValidate unmarshals and validates a claim token.
// See Claim.Validate for details.
func ClaimValidate(claimToken string) error {
	claim, err := wonka.UnmarshalClaim(claimToken)
	if err != nil {
		return err
	}
	return claim.Validate()
}
