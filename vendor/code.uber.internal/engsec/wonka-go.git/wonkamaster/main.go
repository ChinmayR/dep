package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"

	"code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/internal/keyhelper"
	"code.uber.internal/engsec/wonka-go.git/internal/rpc"
	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/handlers"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/middleware"
	"code.uber.internal/engsec/wonka-go.git/wonkamaster/wonkadb"

	configfx "code.uber.internal/go/configfx.git"
	debugfx "code.uber.internal/go/debugfx.git"
	envfx "code.uber.internal/go/envfx.git"
	healthfx "code.uber.internal/go/healthfx.git"
	jaegerfx "code.uber.internal/go/jaegerfx.git"
	maxprocsfx "code.uber.internal/go/maxprocsfx.git"
	runtimefx "code.uber.internal/go/runtimefx.git"
	sentryfx "code.uber.internal/go/sentryfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	systemportfx "code.uber.internal/go/systemportfx.git"
	tallyfx "code.uber.internal/go/tallyfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	zapfx "code.uber.internal/go/zapfx.git"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func main() {
	fx.New(
		// These are all the modules provided by uberfx except galileo.
		// See https://code.uberinternal.com/diffusion/GOUBEAQ/browse/master/uberfx.go
		configfx.Module,
		debugfx.Module,
		envfx.Module,
		healthfx.Module,
		maxprocsfx.Module,
		jaegerfx.Module,
		runtimefx.Module,
		sentryfx.Module,
		servicefx.Module,
		systemportfx.Module,
		tallyfx.Module,
		versionfx.Module,
		zapfx.Module,
		fx.Provide(
			loadConfig,
			newPulloClient,
			newCassandraDB,
		),
		fx.Invoke(run),
	).Run()
}

func loadConfig(c config.Provider) (appConfig, error) {
	var cfg appConfig
	err := c.Get(config.Root).Populate(&cfg)
	return cfg, err
}

func newPulloClient(cfg appConfig, log *zap.Logger, level zap.AtomicLevel) (rpc.PulloClient, error) {
	if len(cfg.PulloConfig) > 0 {
		log.Debug("debug Pullo config found", zap.Any("config", cfg.PulloConfig))
		return rpc.NewMockPulloClient(cfg.PulloConfig, rpc.Logger(log, level)), nil
	}

	return rpc.NewPulloClient(rpc.Logger(log, level))
}

func newCassandraDB(cfg appConfig, log *zap.Logger, metrics tally.Scope) (wonkadb.EntityDB, error) {
	cfg.Cassandra.Logger = log
	cfg.Cassandra.Metrics = metrics
	return wonkadb.NewCassandra(cfg.Cassandra)
}

type runParams struct {
	fx.In

	Lifecycle   fx.Lifecycle
	Logger      *zap.Logger
	Metrics     tally.Scope
	Pullo       rpc.PulloClient
	EntityDB    wonkadb.EntityDB
	Environment envfx.Context
}

func run(p runParams, cfg appConfig) error {
	// TODO(abg): Avoid use of global logger
	zap.ReplaceGlobals(p.Logger)
	log := p.Logger

	port, err := getDynamicHTTPPort(cfg.Port)
	if err != nil {
		log.Error("error finding dynamic port. Using fallback.", zap.Error(err))
	}
	log.Debug("Starting wonka master", zap.Int("port", port))

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	// Fetch private key out of langley
	var masterKeyPath masterKey
	if p.Environment.RuntimeEnvironment == envfx.EnvStaging {
		masterKeyPath = cfg.WonkaMasterKeyStaging
	} else {
		masterKeyPath = cfg.WonkaMasterKey
	}
	rsaKey, eccKey, err := loadPrivateKey(masterKeyPath)
	if err != nil {
		log.Error("error loading wonkamaster private key",
			zap.Error(err),
			zap.Any("expected", masterKeyPath),
		)
		return err
	}

	pubKey := eccKey.PublicKey
	compressedKey := wonka.KeyToCompressed(pubKey.X, pubKey.Y)
	os.Setenv("WONKA_MASTER_ECC_PUB", compressedKey)
	wonka.InitWonkaMasterECC()

	log.Info("server ecc public key",
		zap.Any("compressed", compressedKey),
		zap.Any("path", masterKeyPath),
	)

	//TODO(jesses) replace this with the canonical hostname tagging method in `tally`.
	ms := p.Metrics.Tagged(map[string]string{"hostname": hostname})
	ms.Tagged(map[string]string{"stage": "boot"}).Counter("server").Inc(1)

	userSigners, usshHostCB, err := loadUssh(cfg.UsshUserSigner.TrustedUserCa, cfg.UsshHostSigner.SSHKnownHosts)
	if err != nil {
		log.Error("loading user signer",
			zap.Any("path", cfg.UsshUserSigner.TrustedUserCa),
			zap.Error(err),
		)

		return err
	}

	filter := xhttp.NewFilterChainBuilder().
		AddFilter(xhttp.DefaultFilter).
		AddFilter(middleware.NewRateLimiter(cfg.Rates)).
		Build()
	r := xhttp.NewRouterWithFilter(filter)

	handlerCfg := common.HandlerConfig{
		Metrics:                    ms.Tagged(map[string]string{"source": "handlers"}),
		ECPrivKey:                  eccKey,
		RSAPrivKey:                 rsaKey,
		Ussh:                       userSigners,
		UsshHostSigner:             usshHostCB,
		DB:                         p.EntityDB,
		Pullo:                      p.Pullo,
		Imp:                        cfg.Impersonators,
		Logger:                     log,
		Host:                       hostname,
		Derelicts:                  cfg.Derelicts,
		Launchers:                  cfg.Launchers,
		HoseCheckInterval:          cfg.HoseCheckInterval,
		CertAuthenticationOverride: cfg.CertAuthentiationOverride,
	}
	handlers.SetupHandlers(r, handlerCfg)

	server := &http.Server{Handler: r}
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			// Listen on assigned inbound-port
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				log.Error("error setting up listener", zap.Error(err))
				return err
			}

			go server.Serve(ln)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})

	return nil
}

func getDynamicHTTPPort(fallback int) (int, error) {
	portEnv := os.Getenv("UBER_PORT_HTTP")
	if portEnv == "" {
		return fallback, nil
	}
	portOverride, err := strconv.Atoi(portEnv)
	if err != nil {
		return fallback, fmt.Errorf("error parsing UBER_PORT_HTTP: %v", err)
	}
	return portOverride, nil
}

func loadUssh(userCA, hostCA string) ([]ssh.PublicKey, ssh.HostKeyCallback, error) {
	var usshHost ssh.HostKeyCallback
	var err error

	if hostCA != "" {
		usshHost, err = knownhosts.New(hostCA)
		if err != nil {
			return nil, nil, fmt.Errorf("error loading ssh known hosts file: %v", err)
		}
	}

	var usshUser []ssh.PublicKey
	userCABytes, err := ioutil.ReadFile(userCA)
	if err != nil {
		return nil, nil, err
	}

	in := userCABytes
	for {
		pub, _, _, rest, err := ssh.ParseAuthorizedKey(in)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing ssh key: %v", err)
		}
		usshUser = append(usshUser, pub)
		if len(rest) == 0 {
			break
		}
		in = rest
	}

	return usshUser, usshHost, nil
}

// Fetches secrets out of a langley secrets file
func loadPrivateKey(m masterKey) (*rsa.PrivateKey, *ecdsa.PrivateKey, error) {
	// Open the wonkamaster private key file and read out the contents
	rsaKey, err := keyhelper.New().RSAFromFile(m.PrivatePem)
	if err != nil {
		return nil, nil, err
	}
	eccKey := wonka.ECCFromRSA(rsaKey)

	return rsaKey, eccKey, nil
}
