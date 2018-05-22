package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/keys"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/rpc"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	"code.uber.internal/engsec/wonka-go.git/internal/timehelper"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"github.com/uber-go/tally"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"
)

const (
	wonkaSamplePrefix = "wonkaSample"
)

var (
	allowedEnrollSkew = time.Minute
	// validEntityChars permits alphanumeric + "." and ":". It must end with alphanumeric.
	validEntityChars = regexp.MustCompile("^[A-Za-z0-9]([A-Za-z0-9-_\\.:]*[A-Za-z0-9])?$")
)

type enrollHandler struct {
	log         *zap.Logger
	metrics     tally.Scope
	db          wonkadb.EntityDB
	host        string
	pulloClient rpc.PulloClient
}

func (h enrollHandler) logAndMetrics() (*zap.Logger, tally.Scope) {
	return h.log, h.metrics
}

// newEnrollHandler returns a new handler that serves enroll http requests.
func newEnrollHandler(cfg common.HandlerConfig) xhttp.Handler {
	h := enrollHandler{
		log:         cfg.Logger.With(zap.String("endpoint", "enroll")),
		metrics:     cfg.Metrics.Tagged(map[string]string{"endpoint": "enroll"}),
		db:          cfg.DB,
		host:        cfg.Host,
		pulloClient: cfg.Pullo,
	}

	return h
}

// Request can be authorized by sending a valid claim in one of
// 1. request body
// 2. X-Wonka-Auth header
func (h enrollHandler) isAuthorized(ctx context.Context, req *http.Request, e *wonka.Entity, c *wonka.Claim) (bool, string) {
	authorized := false
	authorizedBy := ""

	allowedGroups := []string{wonka.EnrollerGroup}

	if c != nil {
		// if the user is *in* engineering, we add identity claims to the list of allowed claims.
		if h.pulloClient.IsMemberOf(c.EntityName, "engineering") {
			h.log.Debug("user is in engineering!")
			allowedGroups = append(allowedGroups, []string{c.EntityName, wonka.EveryEntity, wonka.EnrollerGroup}...)
		}
		authorizedBy = c.EntityName
		authorized = true
		if err := c.Check("wonkamaster", allowedGroups); err != nil {
			authorized = false
			h.log.Info("claim check failed",
				zap.Error(err),
				zap.String("destination", c.Destination),
				zap.String("entity", c.EntityName),
				zap.Bool("authorized", authorized),
			)
		}
		h.log.Debug("found claim in request body",
			zap.Bool("authorized", authorized),
			zap.String("entity", authorizedBy),
		)
	} else {
		if wonkaHdr, ok := req.Header["X-Wonka-Auth"]; ok {
			claim, err := wonka.UnmarshalClaim(wonkaHdr[0])
			if err != nil {
				return false, ""
			}
			// if the request contains an X-Wonka-Auth header and the claim is valid, this
			// is an authorized request.
			authorized = true
			authorizedBy = claim.EntityName
			if err := claim.Check("wonkamaster", allowedGroups); err != nil {
				authorized = false
				h.log.Info("claim check failed",
					zap.Error(err),
					zap.String("destination", c.Destination),
					zap.String("entity", c.EntityName),
					zap.Bool("authorized", authorized),
				)
			}
		}
		h.log.Debug("claim check failed, checking X-Wonka-Auth",
			zap.Bool("authorized", authorized),
			zap.String("entity", authorizedBy),
		)
	}

	return authorized, authorizedBy
}

// EnrollHandler enrolls/updates an entity
func (h enrollHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	stopWatch := h.metrics.Timer("server").Start()
	defer stopWatch.Stop()
	w.Header().Set("X-Wonkamaster", h.host)

	h.log = h.log.With(jaegerzap.Trace(ctx))

	var e wonka.EnrollRequest
	// Parse json enrollment message into e
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&e); err != nil {
		writeResponse(w, h, err, wonka.DecodeError, http.StatusBadRequest)
		return
	}

	if e.Entity == nil {
		writeResponse(w, h, errors.New("nill entity"), wonka.ResultRejected, http.StatusBadRequest)
		return
	}

	h.log = h.log.With(
		zap.String("enrollee_entity", e.Entity.EntityName),
		zap.String("location", e.Entity.Location),
	)

	rsaPubKey, err := keys.ParsePublicKey(e.Entity.PublicKey)
	if err != nil {
		writeResponse(w, h, err, wonka.EnrollInvalidPublicKey, http.StatusBadRequest)
		return
	}

	toVerify := fmt.Sprintf("%s<%d>%s", e.Entity.EntityName, e.Entity.Ctime, e.Entity.PublicKey)
	if err := keys.VerifySignature(rsaPubKey, e.Entity.EntitySignature, e.Entity.SigType, toVerify); err != nil {
		writeResponse(w, h, err, wonka.SignatureVerifyError, http.StatusForbidden)
		return
	}

	// if the remote entity has the right claims,
	// authorized == true. In practice this means that an engineer will be able to enroll
	// a new service. If authorized is false, then sample entities can still be
	// enrolled and entities can update themselves.
	authorized, enrolledBy := h.isAuthorized(ctx, req, e.Entity, e.Claim)
	h.log = h.log.With(
		zap.Bool("authorized", authorized),
		zap.String("entity", enrolledBy),
	)

	createTime := time.Unix(int64(e.Entity.Ctime), 0)
	expireTime := time.Unix(int64(e.Entity.Etime), 0)

	if expireTime.Before(e.Entity.CreateTime) {
		h.log = h.log.With(
			zap.Time("ctime", e.Entity.CreateTime),
			zap.Time("etime", e.Entity.ExpireTime),
		)
		writeResponse(w, h, errors.New("Request created after expiration"),
			wonka.EnrollInvalidTime, http.StatusForbidden)
		return
	}

	now := time.Now()
	// is the entity request expired? - Claims are only valid for 60 seconds
	if !timehelper.WithinClockSkew(createTime, now, allowedEnrollSkew) {
		h.log = h.log.With(
			zap.Time("ctime", e.Entity.CreateTime),
			zap.Time("etime", e.Entity.ExpireTime),
			zap.Time("now", now))
		writeResponse(w, h, errTime, wonka.ErrTimeWindow, http.StatusForbidden)
		return

	}

	result, code, err := h.createOrUpdate(ctx, *e.Entity, authorized)
	if err != nil {
		writeResponse(w, h, err, result, code)
		return
	}

	if authorized {
		h.log.Info("service entity enrolled/updated",
			zap.Any("enrolledby", enrolledBy),
		)
	}

	// Now lets try to lookup the registered entity
	dbe, err := h.db.Get(ctx, e.Entity.EntityName)
	if err != nil {
		writeResponse(w, h, errors.New("get entity failed"), wonka.ResultRejected, http.StatusInternalServerError)
		return
	}

	resp := wonka.EnrollResponse{
		Result: wonka.ResultOK,
		Entity: *dbe,
	}

	writeResponse(w, h, nil, resp.Result, http.StatusOK, responseBody(resp))
}

// canCreate returns true if the requester is allowed to create this entity.
func canCreate(e wonka.Entity, authorized bool) error {
	if !validEntityChars.Match([]byte(e.EntityName)) {
		return errors.New(wonka.EnrollInvalidEntity)
	}

	pfx := strings.Split(e.EntityName, ":")
	switch len(pfx) {
	case 1:
		if !authorized {
			return errors.New(wonka.EnrollNotPermitted)
		}
		return nil
	case 2:
		// the only prefix we allow is wonkasample:
		if !strings.EqualFold(pfx[0], wonkaSamplePrefix) {
			return errors.New(wonka.EnrollInvalidEntity)
		}
		return nil
	default:
		// only permit zero or one colons.
		return errors.New(wonka.EnrollNotPermitted)
	}
}

func (h enrollHandler) tryCreate(ctx context.Context, e wonka.Entity, authorized bool) (string, int, error) {
	if err := canCreate(e, authorized); err != nil {
		return err.Error(), http.StatusForbidden, err
	}

	if err := h.db.Create(ctx, &e); err != nil {
		return wonka.ResultRejected, http.StatusInternalServerError, fmt.Errorf("error creating entity: %v", err)
	}

	return "", 0, nil
}

// canUpdate returns true if the requestor is allowed to update this entity.
func canUpdate(e, dbe wonka.Entity) bool {
	return strings.HasPrefix(e.EntityName, wonkaSamplePrefix) ||
		e.PublicKey == dbe.PublicKey
}

func (h enrollHandler) tryUpdate(ctx context.Context, e, dbe wonka.Entity) (string, int, error) {
	if !canUpdate(e, dbe) {
		return wonka.EnrollNotPermitted, http.StatusForbidden, errors.New("no permission to update")
	}

	if err := h.db.Update(ctx, &e); err != nil {
		return wonka.ResultRejected, http.StatusInternalServerError, fmt.Errorf("error updating entity: %v", err)
	}

	return "", 0, nil
}

func (h enrollHandler) createOrUpdate(ctx context.Context, e wonka.Entity, authorized bool) (string, int, error) {
	if strings.EqualFold(e.EntityName, wonka.NullEntity) || strings.EqualFold(e.EntityName, wonka.EveryEntity) {
		return "invalid attempt to register invalid entity", http.StatusBadRequest, errors.New("bad entity name")
	}

	dbe, err := h.db.Get(ctx, e.EntityName)
	if err == wonkadb.ErrNotFound {
		return h.tryCreate(ctx, e, authorized)
	}
	if err != nil {
		return "error looking up entity", http.StatusInternalServerError, err
	}
	return h.tryUpdate(ctx, e, *dbe)
}

func (h enrollHandler) createEntityShadow(ctx context.Context, e wonka.Entity) (string, int, error) {
	if err := h.db.Create(ctx, &e); err != nil {
		h.log.Error("failed to shadow write create", zap.Error(err))
		return wonka.ResultRejected, http.StatusInternalServerError, errors.New("error creating entity")
	}
	return "", 0, nil
}

func (h enrollHandler) updateEntityShadow(ctx context.Context, e wonka.Entity) (string, int, error) {
	if err := h.db.Update(ctx, &e); err != nil {
		h.log.Error("failed to shadow write update", zap.Error(err))
		return wonka.ResultRejected, http.StatusInternalServerError, errors.New("error update entity")
	}
	return "", 0, nil
}
