package galileo

// GetAllowedEntities accepts an error returned by Galileo.AuthenticateIn and
// returns a list of entities which would have been allowed to make that
// request.
func GetAllowedEntities(err error) []string {
	aerr, ok := err.(*authError)
	if !ok || len(aerr.AllowedEntities) == 0 {
		return []string{EveryEntity}
	}
	return aerr.AllowedEntities
}

type authError struct {
	AllowedEntities []string
	Reason          error
}

var _ error = (*authError)(nil)

func (e *authError) Error() string {
	return e.Reason.Error()
}
