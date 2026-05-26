// Package server exposes the exporter's HTTP endpoints.
package server

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HealthChecker reports process health and readiness.
type HealthChecker interface {
	Healthy() bool
	Ready() bool
}

// Handler builds the HTTP mux: /metrics, /healthz, /readyz.
func Handler(reg *prometheus.Registry, hc HealthChecker) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", probe(hc.Healthy))
	mux.HandleFunc("/readyz", probe(hc.Ready))
	return mux
}

// New builds an *http.Server with sane timeouts wrapping Handler.
func New(addr string, reg *prometheus.Registry, hc HealthChecker) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           Handler(reg, hc),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func probe(ok func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if ok() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}
}
