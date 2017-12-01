package main

import (
	"flag"
	"fmt"
	"net"

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

	w := &wonkad{log: log}

	if err := w.setupListeners(*unixSocket, *loopbackAddr); err != nil {
		w.log.Fatal("setting up listeners", zap.Error(err))
	}

	if err := w.listenAndServe(); err != nil {
		log.Fatal("wonkad failed", zap.Error(err))
	}
}
