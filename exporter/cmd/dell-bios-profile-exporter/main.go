// Command dell-bios-profile-exporter scrapes the Dell BIOS System Profile via
// host-local racadm and exposes it as Prometheus metrics.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cicdteam/dell-bios-profile-exporter/internal/collector"
	"github.com/cicdteam/dell-bios-profile-exporter/internal/config"
	"github.com/cicdteam/dell-bios-profile-exporter/internal/poller"
	"github.com/cicdteam/dell-bios-profile-exporter/internal/racadm"
	"github.com/cicdteam/dell-bios-profile-exporter/internal/server"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(2)
	}

	if *healthcheck {
		os.Exit(runHealthcheck(cfg.ListenPort))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	reg := prometheus.NewRegistry()

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dell_bios_exporter_build_info",
		Help: "Exporter build info; value is always 1.",
	}, []string{"version", "go_version"})
	buildInfo.WithLabelValues(version, runtime.Version()).Set(1)
	reg.MustRegister(buildInfo)

	errs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dell_bios_racadm_errors_total",
		Help: "Count of racadm failures by reason.",
	}, []string{"node", "reason"})
	reg.MustRegister(errs)

	cache := &collector.Cache{}
	reg.MustRegister(collector.New(cfg.NodeName, cfg.TargetProfile, cache))

	client := racadm.NewClient(racadm.ExecRunner{}, cfg.NsenterPath, cfg.RacadmPath, cfg.RequestTimeout)
	p := poller.New(client, cache, errs, poller.Options{
		Node:              cfg.NodeName,
		Target:            cfg.TargetProfile,
		PollInterval:      cfg.PollInterval,
		SysinfoInterval:   cfg.SysinfoInterval,
		HealthMaxStale:    2*cfg.PollInterval + cfg.RequestTimeout,
		ReadyzMaxFailures: cfg.ReadyzMaxFailures,
		Logger:            logger,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go p.Run(ctx)

	srv := server.New(fmt.Sprintf(":%d", cfg.ListenPort), reg, p)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("shutdown complete")
}

func runHealthcheck(port int) int {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://localhost:%d/healthz", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
