package galileo

// TODO(pmoody): remove the static functions that require a global galileo instance.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal"

	wonka "code.uber.internal/engsec/wonka-go.git"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

// newGalileo creates a new Galileo instance that you'll have to manage
func newGalileo(cfg Configuration, w wonka.Wonka) (Galileo, error) {
	if cfg.EnforcePercentage < 0 {
		cfg.Logger.Error("enforce_percentage should be from [0.0, 1.0]")
		cfg.EnforcePercentage = 0
	}

	if cfg.EnforcePercentage > 1 {
		cfg.Logger.Error("enforce_percentage should be from [0.0, 1.0]")
		cfg.EnforcePercentage = float32((int(cfg.EnforcePercentage) % 100)) / 100.0
	}

	// seeding with time.Now() is fine because this doesn't require any
	// sort of cryptographic security
	rand.Seed(int64(time.Now().Nanosecond()))

	g := &uberGalileo{
		serviceName:       cfg.ServiceName,
		allowedEntities:   cfg.AllowedEntities,
		metrics:           cfg.Metrics,
		w:                 w,
		cachedClaims:      make(map[string]*wonka.Claim, 0),
		claimLock:         &sync.Mutex{},
		skippedEntities:   make(map[string]skippedEntity, 1),
		skipLock:          &sync.RWMutex{},
		log:               cfg.Logger,
		enforcePercentage: cfg.EnforcePercentage,
		disabled:          0,
		endpointCfg:       cfg.Endpoints,
		tracer:            cfg.Tracer,
	}

	go g.checkDisableStatus(wonka.WonkaMasterPublicKey)

	return g, nil
}

// initLogAndMetrics initializes the logger and metrics scope.
// Values from configuration object will be used, if given. Otherwise, default
// values will be used so we don't force extra configuration on consumers.
// Either way, logger and metrics will be scoped to component:galileo
// and by the given service name.
func (cfg *Configuration) initLogAndMetrics() error {
	if cfg.Metrics == nil {
		cfg.Metrics = tally.NoopScope
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.L()
	}
	cfg.Logger = cfg.Logger.With(zap.Namespace("galileo"), zap.String("entity", cfg.ServiceName))
	cfg.Metrics = cfg.Metrics.Tagged(map[string]string{"component": "galileo", "entity": cfg.ServiceName})
	return nil
}

// CreateWithContext creates a new Galileo instance for cfg.ServiceName and uses
// the provided context to enroll that instance in Wonka. Enroll updates the
// allowed entities.
func CreateWithContext(ctx context.Context, cfg Configuration) (Galileo, error) {
	if err := cfg.initLogAndMetrics(); err != nil {
		return nil, err
	}

	// isDisabled calls some logging and metrics handlers so these need to be initialized first.
	if cfg.Disabled {
		return &uberGalileo{
			disabled: 1,
			metrics:  cfg.Metrics,
			log:      cfg.Logger,
		}, nil
	}

	// Require ServiceName field
	if cfg.ServiceName == "" {
		return nil, errors.New("Configuration must have ServiceName parameter set")
	}

	// Initialize opentracer
	if cfg.Tracer == nil {
		cfg.Tracer = opentracing.GlobalTracer()
	}

	if _, isNoop := cfg.Tracer.(opentracing.NoopTracer); isNoop {
		return nil, errors.New("jaeger must be initialized before calling galileo")
	}

	wonkaCfg := wonka.Config{
		EntityName:     cfg.ServiceName,
		PrivateKeyPath: cfg.PrivateKeyPath,
		Metrics:        cfg.Metrics,
		Logger:         cfg.Logger,
		Tracer:         cfg.Tracer,
	}

	w, err := wonka.InitWithContext(ctx, wonkaCfg)
	if err != nil {
		return nil, fmt.Errorf("error initializing wonka for galileo: %v", err)
	}

	if cfg.PrivateKeyPath != "" && len(cfg.AllowedEntities) > 0 {
		if _, err := w.Enroll(ctx, "none" /* location */, cfg.AllowedEntities); err != nil {
			return nil, fmt.Errorf("error enrolling: %v", err)
		}
	}

	// finally, create our galileo instance
	g, err := newGalileo(cfg, w)
	if err != nil {
		return nil, fmt.Errorf("error creating galileo instance: %v", err)
	}

	cfg.Logger.Debug("galileo: instance created", zap.String("name", g.Name()))

	cfg.Metrics.Tagged(map[string]string{
		"stage":   "initialized",
		"version": Version,
	}).Counter("galileo-go").Inc(1)

	return g, nil
}

// Create creates a new Galileo instance and uses the Background context for
// enrollment.
func Create(cfg Configuration) (Galileo, error) {
	return CreateWithContext(context.Background(), cfg)
}

// GetClaim returns the service auth claim (wonka token) attached to this context.
func GetClaim(ctx context.Context) (*wonka.Claim, error) {
	claimAttr, err := internal.ClaimFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if claimAttr == "" {
		return nil, errors.New("galileo: no wonka claim found")
	}

	claim, err := wonka.UnmarshalClaim(claimAttr)
	if err != nil {
		return nil, errors.New("galileo: error unmarshailing wonka claim")
	}

	return claim, nil
}

// GetLogger returns the zap.Logger associated with this instance of Galileo.
// If no instance was configured, it returns the global logger.
func GetLogger(g Galileo) *zap.Logger {
	u, ok := g.(*uberGalileo)
	if ok {
		return u.log
	}
	return zap.L()
}

func shouldEnforce(enforcePercentage float32) bool {
	if enforcePercentage == 0 {
		return false
	}
	if enforcePercentage == 1 {
		return true
	}

	// this is seeded at package initialization.
	return rand.Float32() <= enforcePercentage
}

// ctxClaimKeyType is used to retrieve a claim from a context.Context.
type ctxClaimKeyType struct{}

var _ctxClaimKey = &ctxClaimKeyType{}

// WithClaim sets an explicit claim for an outgoing request on the given context.
// Multiple claims can be set with a comma-separated string.
//
//  galileo.WithClaim(ctx, "AD:engineering")
//  galileo.WithClaim(ctx, "AD:engineering, AD:engsec")
func WithClaim(ctx context.Context, claim string) context.Context {
	return context.WithValue(ctx, _ctxClaimKey, claim)
}
