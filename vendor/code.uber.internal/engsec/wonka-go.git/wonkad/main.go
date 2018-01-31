package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/uber-go/tally"
	"golang.org/x/crypto/ssh/agent"

	"code.uber.internal/engsec/wonka-go.git"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	unixSocket   = flag.String("unix_socket", "/var/run/wonkad.sock", "local socket to listen for rqeuests")
	loopbackAddr = flag.String("loopback_addr", wonka.WonkadTCPAddress,
		"listen for connections at this address on the loopback interface")
	verbose    = flag.Bool("v", false, "verbose")
	sshdConfig = flag.String("sshd_config", "/etc/ssh/sshd_config", "where to the find the system sshd_config")
)

type wonkad struct {
	unixListener net.Listener
	tcpListener  net.Listener
	log          *zap.Logger
	wonka        wonka.Wonka
	host         string
}

func main() {
	flag.Parse()

	logCfg := zap.NewProductionConfig()
	if *verbose {
		logCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	log, err := logCfg.Build()
	if err != nil {
		panic(fmt.Errorf("failed to set up logger: %v", err))
	}
	defer log.Sync()

	zap.ReplaceGlobals(log)

	uwonka, err := createWonka(log)
	if err != nil {
		log.Fatal("creating wonka", zap.Error(err))
	}
	defer wonka.Close(uwonka)
	w := &wonkad{
		log:   log,
		wonka: uwonka,
		host:  uwonka.EntityName(),
	}

	if err := w.setupListeners(*unixSocket, *loopbackAddr); err != nil {
		w.log.Fatal("setting up listeners", zap.Error(err))
	}

	if err := w.listenAndServe(); err != nil {
		log.Fatal("wonkad failed", zap.Error(err))
	}
}

func createWonka(log *zap.Logger) (wonka.Wonka, error) {
	log.Debug("reading sshd config", zap.Any("sshd_config", sshdConfig))
	cert, privKey, err := usshHostCert(log, *sshdConfig)
	if err != nil {
		return nil, fmt.Errorf("error loading host key: %v", err)
	}

	log.Debug("starting ssh agent")
	a := agent.NewKeyring()
	if err := a.Add(agent.AddedKey{PrivateKey: privKey, Certificate: cert}); err != nil {
		return nil, fmt.Errorf("error adding keys to agent: %v", err)
	}

	// now we're ready to talk to wonka.
	host := cert.ValidPrincipals[0]
	cfg := wonka.Config{
		EntityName: host,
		Agent:      a,
		Logger:     log,
		Metrics:    tally.NoopScope,
	}
	w, err := wonka.Init(cfg)
	if err != nil {
		return nil, fmt.Errorf("error initializing wonka: %v", err)
	}
	return w, nil
}
