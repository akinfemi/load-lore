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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/netem"
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
	// netem sets the network profile's delay, or a loss or partition fault
	// on top of it, in the network namespace it shares: see netemCmd.
	// netprobe measures the round trip between two containers, for the
	// network profile: netprobe echo, or netprobe rtt <host:port> <n>,
	// which prints the median in microseconds.
	if len(os.Args) > 1 && os.Args[1] == "netprobe" {
		if err := netprobeCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "synthetic netprobe:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "netem" {
		if err := netemCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "synthetic netem:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "synthetic:", err)
		os.Exit(1)
	}
}

func netprobeCmd(args []string) error {
	switch {
	case len(args) == 1 && args[0] == "echo":
		return netem.Echo(netem.ProbePort)
	case len(args) == 3 && args[0] == "rtt":
		n, err := strconv.Atoi(args[2])
		if err != nil || n < 1 {
			return fmt.Errorf("bad count %q", args[2])
		}
		d, err := netem.RTT(args[1], n)
		if err != nil {
			return err
		}
		fmt.Println(d.Microseconds())
		return nil
	}
	return errors.New("usage: netprobe echo | netprobe rtt <host:port> <n>")
}

func netemCmd(args []string) error {
	usage := errors.New("usage: netem base <delay_us> | netem loss <sport> <percent> <delay_us> | netem cut <dport,...|-> <ip,...|-> <delay_us>")
	if len(args) < 2 {
		return usage
	}
	delay, err := strconv.Atoi(args[len(args)-1])
	if err != nil || delay < 0 || delay > 10_000_000 {
		return fmt.Errorf("bad delay %q", args[len(args)-1])
	}
	switch {
	case args[0] == "base" && len(args) == 2:
		return netem.Base(delay)
	case args[0] == "cut" && len(args) == 4:
		var ports []int
		if args[1] != "-" {
			for _, f := range strings.Split(args[1], ",") {
				port, err := strconv.Atoi(f)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("bad port %q", f)
				}
				ports = append(ports, port)
			}
		}
		var ips []net.IP
		if args[2] != "-" {
			for _, f := range strings.Split(args[2], ",") {
				ip := net.ParseIP(f)
				if ip == nil || ip.To4() == nil {
					return fmt.Errorf("bad IPv4 address %q", f)
				}
				ips = append(ips, ip)
			}
		}
		if len(ports)+len(ips) == 0 {
			return errors.New("cut: nothing to cut")
		}
		return netem.Cut(ports, ips, delay)
	case args[0] == "loss" && len(args) == 4:
		sport, err := strconv.Atoi(args[1])
		if err != nil || sport < 1 || sport > 65535 {
			return fmt.Errorf("bad port %q", args[1])
		}
		pct, err := strconv.ParseFloat(args[2], 64)
		if err != nil || pct <= 0 || pct > 100 {
			return fmt.Errorf("bad percent %q", args[2])
		}
		return netem.Apply(sport, pct, delay)
	}
	return usage
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
	// A worker also takes jobs from its queue; it still serves metrics and
	// health checks over HTTP.
	workerDone := make(chan error, 1)
	if cfg.Role == synthetic.RoleWorker {
		go func() { workerDone <- svc.RunWorker(ctx) }()
		slog.Info("synthetic worker taking jobs", "queue", cfg.Consume.Target, "concurrency", cfg.Consume.Concurrency)
	}

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	stop()
	if cfg.Role == synthetic.RoleWorker {
		// Jobs already taken run to the end.
		select {
		case err := <-workerDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("worker stopped", "error", err)
			}
		case <-time.After(10 * time.Second):
		}
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
