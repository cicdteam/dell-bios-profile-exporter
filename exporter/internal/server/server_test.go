package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// HealthStub returns fixed health/readiness states.
type HealthStub struct {
	healthy bool
	ready   bool
}

func (s HealthStub) Healthy() bool { return s.healthy }
func (s HealthStub) Ready() bool   { return s.ready }

func newServer(t *testing.T, hc HealthChecker) http.Handler {
	t.Helper()
	reg := prometheus.NewRegistry()
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe_metric", Help: "x"})
	g.Set(1)
	reg.MustRegister(g)
	return Handler(reg, hc)
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newServer(t, HealthStub{healthy: true, ready: true})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); !contains(body, "probe_metric") {
		t.Errorf("/metrics body missing probe_metric: %q", body)
	}
}

func TestHealthz(t *testing.T) {
	for _, tc := range []struct {
		healthy bool
		want    int
	}{{true, 200}, {false, 503}} {
		srv := newServer(t, HealthStub{healthy: tc.healthy})
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rr.Code != tc.want {
			t.Errorf("/healthz healthy=%v status = %d, want %d", tc.healthy, rr.Code, tc.want)
		}
	}
}

func TestReadyz(t *testing.T) {
	for _, tc := range []struct {
		ready bool
		want  int
	}{{true, 200}, {false, 503}} {
		srv := newServer(t, HealthStub{ready: tc.ready})
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rr.Code != tc.want {
			t.Errorf("/readyz ready=%v status = %d, want %d", tc.ready, rr.Code, tc.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
