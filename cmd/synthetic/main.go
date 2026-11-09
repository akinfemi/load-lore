// Command synthetic is the loadlore/synthetic container: a configurable
// stand-in for an application service. Its config comes from the
// LOADLORE_SYNTHETIC_CONFIG env var or the file named by
// LOADLORE_SYNTHETIC_CONFIG_FILE.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/akinfemi/load-lore/internal/synthetic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "synthetic:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := synthetic.LoadConfig()
	if err != nil {
		return err
	}
	svc, err := synthetic.New(cfg)
	if err != nil {
		return err
	}
	defer svc.Close()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("synthetic service listening", "service", cfg.Service, "replica", cfg.Replica, "addr", cfg.Listen)

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
