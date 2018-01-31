package galileo

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"code.uber.internal/engsec/galileo-go.git/internal"
	"code.uber.internal/engsec/galileo-go.git/internal/atomic"
	"code.uber.internal/engsec/galileo-go.git/internal/claimtools"
	"code.uber.internal/engsec/galileo-go.git/internal/contexthelper"
	"code.uber.internal/engsec/galileo-go.git/internal/telemetry"

	"code.uber.internal/engsec/wonka-go.git"
	"github.com/opentracing/opentracing-go"
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

	if len(cfg.ServiceAliases) == 0 {
		cfg.ServiceAliases = []string{cfg.ServiceName}
	}

	// seeding with time.Now() is fine because this doesn't require any
	// sort of cryptographic security
	rand.Seed(int64(time.Now().Nanosecond()))

	inboundCache, err := claimtools.NewInboundCache(cfg.Cache)
	if err != nil {
		return nil, err
	}
	g := &uberGalileo{
		serviceName:       cfg.ServiceName,
		serviceAliases:    cfg.ServiceAliases,
		allowedEntities:   cfg.AllowedEntities,
		metrics:           cfg.Metrics,
		w:                 w,
		cachedClaims:      make(map[string]*wonka.Claim, 0),
		claimLock:         &sync.Mutex{},
		skippedEntities:   make(map[string]skippedEntity, 1),
		skipLock:          &sync.RWMutex{},
		log:               cfg.Logger,
		enforcePercentage: atomic.NewFloat64(float64(cfg.EnforcePercentage)),
		disabled:          false,
		endpointCfg:       cfg.Endpoints,
		tracer:            cfg.Tracer,
		inboundClaimCache: inboundCache,
	}

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
	cfg.Logger = cfg.Logger.With(
		zap.Namespace("galileo"),
		zap.String("entity", cfg.ServiceName),
		zap.String("version", internal.LibraryVersion()),
	)
	cfg.Metrics = cfg.Metrics.Tagged(map[string]string{
		"component": "galileo",
		"entity":    telemetry.SanitizeEntityName(cfg.ServiceName),
		// Override host for Galileo's scope in case caller set per-host metrics
		// because Galileo already has large metric cardinality.
		"host":           "global",
		"metricsversion": telemetry.MetricsVersion,
	})
	return nil
}

// CreateWithContext creates a new Galileo instance for cfg.ServiceName and
// passes provided context to Wonka initialization.
func CreateWithContext(ctx context.Context, cfg Configuration) (Galileo, error) {
	if err := cfg.initLogAndMetrics(); err != nil {
		return nil, err
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

	cfg.Metrics.Tagged(map[string]string{
		"language": "go",
		"galileo":  internal.Version,
		"wonka":    wonka.Version,
	}).Counter("version").Inc(1)

	// isDisabled calls some logging and metrics handlers so these need to be initialized first.
	if cfg.Disabled {
		cfg.Logger.Debug("galileo instance created",
			zap.Bool("disabled", true),
			zap.String("name", cfg.ServiceName),
		)
		return &uberGalileo{
			disabled:    true,
			serviceName: cfg.ServiceName,
			metrics:     cfg.Metrics,
			log:         cfg.Logger,
			tracer:      cfg.Tracer,
		}, nil
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

	// finally, create our galileo instance
	g, err := newGalileo(cfg, w)
	if err != nil {
		return nil, fmt.Errorf("error creating galileo: %v", err)
	}

	cfg.Logger.Debug("galileo instance created",
		zap.Bool("disabled", false),
		zap.String("name", g.Name()),
	)

	cfg.Metrics.Tagged(map[string]string{"stage": "initialized"}).Counter("running").Inc(1)

	return g, nil
}

// Create creates a new Galileo instance using Background context.
func Create(cfg Configuration) (Galileo, error) {
	return CreateWithContext(context.Background(), cfg)
}

// GetClaim returns the service auth claim (wonka token) attached to this
// context, and removes it from the context.
func GetClaim(ctx context.Context) (*wonka.Claim, error) {
	return contexthelper.ClaimFromContext(ctx)
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

func shouldEnforce(enforcePercentage float64) bool {
	if enforcePercentage == 0 {
		return false
	}
	if enforcePercentage == 1 {
		return true
	}

	// this is seeded at package initialization.
	return rand.Float64() <= enforcePercentage
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
