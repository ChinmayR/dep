package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

const _allowedSkew = time.Minute

var errTime = errors.New("outside of allowed time window")

func validClaimsFromRequest(ctx context.Context, log *zap.Logger, pc rpc.PulloClient, req wonka.ClaimRequest, reqType claimRequestType) (string, error) {
	claimsApproved := make(map[string]struct{})
	claimsRequested := strings.Split(req.Claim, ",")

	for _, c := range claimsRequested {
		if strings.EqualFold(c, wonka.EveryEntity) {
			claimsApproved[wonka.EveryEntity] = struct{}{}
		}

		if strings.EqualFold(c, req.EntityName) {
			claimsApproved[req.EntityName] = struct{}{}
		}

		// null entity is a no-no
		if strings.EqualFold(c, wonka.NullEntity) {
			return "", errors.New("null entity requested")
		}
	}

	switch reqType {
	case userClaim:
		var err error
		claimsApproved, err = userClaimsFromReq(ctx, claimsRequested, claimsApproved, req, pc, log)
		if err != nil {
			return "", err
		}
	case serviceClaim:
		var err error
		claimsApproved, err = serviceClaimsFromReq(claimsRequested, claimsApproved, req, log)
		if err != nil {
			return "", err
		}
	default:
		return "", errors.New("invalid request type")
	}

	if len(claimsApproved) == 0 {
		return "", errors.New("no allowed claims found")
	}

	// now we just add the two claims that every entity can get.
	// these might already be set, we don't care.
	claimsApproved[wonka.EveryEntity] = struct{}{}
	claimsApproved[req.EntityName] = struct{}{}

	claimSlice := make([]string, 0, len(claimsApproved))
	for k := range claimsApproved {
		claimSlice = append(claimSlice, k)
	}
	sort.Strings(claimSlice)

	return strings.Join(claimSlice, ","), nil
}

func userClaimsFromReq(ctx context.Context, claimsRequested []string, approved map[string]struct{}, req wonka.ClaimRequest, pc rpc.PulloClient, log *zap.Logger) (map[string]struct{}, error) {
	entityGroups, err := pc.GetGroupsForUser(ctx, req.EntityName)
	if err != nil {
		return approved, err
	}
	if len(entityGroups) == 0 {
		log.Warn("no groups for user")
		return approved, nil
	}

	// Are there AD: claims in this requested claim set?
	for _, c := range claimsRequested {
		c = strings.ToLower(c)
		if isADGroupClaim(c) {
			if _, ok := entityGroups[c]; ok {
				approved[c] = struct{}{}
			}
		}
	}

	return approved, nil
}

func serviceClaimsFromReq(claimsRequested []string, approved map[string]struct{}, req wonka.ClaimRequest, log *zap.Logger) (map[string]struct{}, error) {
	groups := make(map[string]struct{}, 1)

	// TODO(pmoody): remove this. this is only for the old wonka-native groups which
	// we don't support anymore. T1049923
	for _, c := range claimsRequested {
		// Skip AD: claims - we're looking for wonka-native claims
		if isADGroupClaim(c) {
			log.Warn("invalid AD membership request from service",
				zap.String("entity", req.EntityName), zap.String("claim", c))
			continue
		}

		// Is this entity a member of the requested native wonka group? Check list
		if isEntityInDeprecatedGroup(c, req.EntityName) {
			// Add this found native group membership entry to the map
			groups[c] = struct{}{}
		}
	}

	for _, c := range claimsRequested {
		if _, ok := groups[c]; ok || strings.EqualFold(c, req.EntityName) || strings.EqualFold(c, wonka.EveryEntity) {
			approved[c] = struct{}{}
		}
	}
	return approved, nil
}

func isADGroupClaim(c string) bool {
	trimmed := strings.TrimLeftFunc(c, unicode.IsSpace)
	return len(trimmed) >= 3 && strings.EqualFold("ad:", trimmed[:3])
}

func isPersonnelClaim(c string) bool {
	// Anything containing '@' is a personnel entity as far as we are concerned - neither services
	// nor hosts should contain this character.
	return strings.ContainsRune(c, '@')
}

func tag(key, value string) map[string]string {
	return map[string]string{key: value}
}

func buildTags(tags ...map[string]string) map[string]string {
	newMap := make(map[string]string, len(tags))
	for _, tag := range tags {
		for k, v := range tag {
			newMap[k] = v
		}
	}
	return newMap
}

func tagCounter(m tally.Scope, counter string, tags ...map[string]string) {
	if tags == nil {
		return
	}
	m.Tagged(buildTags(tags...)).Counter(counter).Inc(1)
}

// tagError is specifically for when we can't return an error result
// to the client due to error while marshaling response.
func tagError(m tally.Scope, err error) {
	tagCounter(m, "call", tag("result", "error"), tag("error", err.Error()))
}

func tagSuccess(m tally.Scope) {
	tagCode(m, "success", http.StatusOK)
}

func tagCode(m tally.Scope, result string, code int) {
	tagCounter(m, "call", tag("result", result), tag("status_code", strconv.Itoa(code)))
}

type wonkaMasterHandler interface {
	logAndMetrics() (*zap.Logger, tally.Scope)
}

// responseOptions are configurable properties of the logs, metrics, and http
// response created in writeResponse.
type responseOptions struct {
	callerSkip   int
	responseBody interface{}
}

// responseOption allows optional configuration of the behavior of writeResponse
type responseOption func(*responseOptions)

// responseBody lets you override the default response from writeResponse. The
// given object will be serialized as json.
func responseBody(body interface{}) responseOption {
	return func(o *responseOptions) {
		o.responseBody = body
	}
}

// addCallerSkip controls which file and line are added to the log message as
// log.caller by zap. It is named after the zap.Option with the same behavior.
func addCallerSkip(skip int) responseOption {
	return func(o *responseOptions) {
		o.callerSkip += skip
	}
}

func writeResponse(w http.ResponseWriter, h wonkaMasterHandler, err error, result string, code int, opts ...responseOption) {
	o := responseOptions{
		callerSkip: 1, // for writeResponse function itself
	}
	for _, opt := range opts {
		opt(&o)
	}

	log, m := h.logAndMetrics()

	l := log.WithOptions(zap.AddCallerSkip(o.callerSkip)).With(
		zap.String("result", result),
		zap.Int("status_code", code),
	)

	if err != nil {
		l = l.With(zap.Error(err))
	}
	l.Warn("writeResponse")

	tagCode(m, result, code)

	w.WriteHeader(code)
	if o.responseBody != nil {
		xhttp.RespondWithJSON(w, o.responseBody)
		return
	}

	if err := xhttp.RespondWithJSON(w, wonka.GenericResponse{Result: result}); err != nil {
		tagError(m, err)
		l.Warn("error writing response", zap.Error(err))
	}
}

func writeResponseForWonkaDBError(w http.ResponseWriter, h wonkaMasterHandler, err error, dbMethod string) {
	res := wonka.LookupServerError
	code := http.StatusInternalServerError
	if err == wonkadb.ErrNotFound {
		res = wonka.ClaimEntityUnknown
		code = http.StatusNotFound
	}
	writeResponse(w, h, fmt.Errorf("error on entity %s: %v", dbMethod, err), res, code, addCallerSkip(1))
}

// authenticateCertificate unmarshals the certificate from b and checks that the certificate is valid and that
// the provided entity name matches the entity name in the certificate.
func authenticateCertificate(b []byte, entity string, o *common.CertAuthOverride, log *zap.Logger) (*wonka.Certificate, error) {
	cert, err := wonka.UnmarshalCertificate(b)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling cert reply: %v", err)
	}

	if cert.EntityName != entity {
		return nil, fmt.Errorf("certificate entity %q does not match requesting entity %q",
			cert.EntityName, entity)
	}

	if err := cert.ValidateSignature(); err != nil {
		return nil, err
	}

	now := time.Now()
	if cert.NotYetValid(now.Add(_allowedSkew)) {
		return nil, errors.New("certificate is not yet valid")
	}

	if cert.Expired(now.Add(-_allowedSkew)) {
		if o == nil {
			return nil, errors.New("certificate has expired")
		}

		g := o.Grant
		if now.After(g.EnforceUntil) {
			return nil, errors.New("certificate has expired")
		}

		signed := time.Unix(int64(cert.ValidAfter), 0)
		if !signed.After(g.SignedAfter) || !signed.Before(g.SignedBefore) {
			return nil, errors.New("certificate has expired")
		}

		log.Info("Authenticating expired certificate due to configured grace period")
	}

	return cert, nil
}
