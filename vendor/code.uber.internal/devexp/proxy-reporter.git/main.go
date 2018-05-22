package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"code.uber.internal/devexp/proxy-reporter.git/handler"

	uberfx "code.uber.internal/go/uberfx.git"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	fx.New(
		uberfx.Module,
		fx.Invoke(func(
			logger *zap.Logger,
			p config.Provider,
			lc fx.Lifecycle,
			scope tally.Scope) error {
			m, err := handler.New(scope, logger)
			if err != nil {
				return err
			}

			srv := http.Server{Handler: m}
			port := p.Get("http").Get("port").String()
			errCh := make(chan error, 1)

			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					ln, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
					if err != nil {
						return fmt.Errorf("error starting mux on port %s: %v", port, err)
					}
					go func() {
						err := srv.Serve(ln)
						if err != http.ErrServerClosed {
							logger.Error("error serving on port", zap.Error(err))
						}
						errCh <- err
					}()
					logger.Info("started HTTP server on haproxy port", zap.Stringer("port", ln.Addr()))
					return nil
				},
				OnStop: func(ctx context.Context) error {
					if err := srv.Shutdown(ctx); err != nil {
						return err
					}
					if err := <-errCh; err != http.ErrServerClosed {
						return err
					}
					return nil
				},
			})
			return nil
		}),
	).Run()
}
