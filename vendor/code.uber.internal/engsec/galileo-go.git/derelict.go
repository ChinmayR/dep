package galileo

import (
	"net/http"

	wonka "code.uber.internal/engsec/wonka-go.git"
)

// DerelictGalileo extends Galileo interface with IsDerelict method to check for
// services that are not expected decorate requests with Wonka auth baggage.
type DerelictGalileo interface {
	Galileo
	IsDerelict(svc string) bool
}

var _ DerelictGalileo = (*uberGalileo)(nil)

// IsDerelictHttp returns true if the service identified in x-uber-source header
// of an http request is considered derelict, i.e. is not expected to
// decorate requests with Wonka auth baggage.
// Even though the name violates golint, we cannot rename because it is now part
// of the ^1 public Galileo API. arc is configured to ignore this violation.
func IsDerelictHttp(g Galileo, r *http.Request) bool {
	dg, ok := g.(DerelictGalileo)
	if !ok {
		return false
	}

	svc := r.Header.Get("x-uber-source")
	if svc == "" {
		GetLogger(dg).Debug("empty x-uber-source")
	}

	return dg.IsDerelict(svc)
}

// IsDerelict returns true if the service identified by the given string is not
// expected to decorate requests with Wonka auth baggage.
func (u *uberGalileo) IsDerelict(entity string) bool {
	return wonka.IsDerelict(u.w, entity)
}
