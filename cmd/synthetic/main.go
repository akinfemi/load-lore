// Command synthetic is the loadlore/synthetic container: a configurable
// stand-in for an application service. Its config comes from the
// LOADLORE_SYNTHETIC_CONFIG env var or the file named by
// LOADLORE_SYNTHETIC_CONFIG_FILE.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/synthetic"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	// bench is the host speed benchmark loadl's pre-flight runs.
	if len(os.Args) > 1 && os.Args[1] == "bench" {
		r, err := bench.Run(context.Background(), time.Second)
		if err != nil {
			fmt.Fprintln(os.Stderr, "synthetic bench:", err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(r)
		return
	}
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

// healthcheck is the container health check: the image has no shell or
// curl, so the binary checks itself. Healthy means every dependency answers.
func healthcheck() int {
	addr := ":8080"
	if cfg, err := synthetic.LoadConfig(); err == nil {
		addr = cfg.Listen
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/readyz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, resp.Status)
		return 1
	}
	return 0
}
