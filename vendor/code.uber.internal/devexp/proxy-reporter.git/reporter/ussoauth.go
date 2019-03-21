package reporter

import "code.uber.internal/engsec/usso-cli.git/ussotoken"

// UssoAuth defines an interface to fetch token for auth
type UssoAuth interface {
	GetTokenForDomain(domain string) (string, error)
}

// OfflineToken is an implementation of UssoAuth to allow
// fetching offline token for a domain from a ussh cert
type OfflineToken struct{}

// GetTokenForDomain converts ussh cert to an offline token
// for a given domain
func (ot OfflineToken) GetTokenForDomain(domain string) (string, error) {
	ussh, err := ussotoken.NewUSSH()
	if err != nil {
		return "", err
	}
	offlineTokenRes, err := ussh.GetOfflineToken(domain)
	if err != nil {
		return "", err
	}
	return offlineTokenRes.Token, nil
}
