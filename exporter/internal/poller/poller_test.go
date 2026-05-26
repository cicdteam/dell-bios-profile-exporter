package poller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cicdteam/dell-bios-profile-exporter/internal/collector"
	"github.com/cicdteam/dell-bios-profile-exporter/internal/racadm"
)

// ClientFake serves scripted profile/sysinfo responses.
type ClientFake struct {
	profile      string
	profileErr   error
	info         racadm.SysInfo
	infoErr      error
	profileCalls int
	infoCalls    int
}

func (f *ClientFake) SysProfile(context.Context) (string, error) {
	f.profileCalls++
	return f.profile, f.profileErr
}

func (f *ClientFake) SysInfo(context.Context) (racadm.SysInfo, error) {
	f.infoCalls++
	return f.info, f.infoErr
}

func newTestPoller(c ProfileClient) (*Poller, *prometheus.CounterVec, *collector.Cache) {
	cache := &collector.Cache{}
	errs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dell_bios_racadm_errors_total", Help: "x",
	}, []string{"node", "reason"})
	p := New(c, cache, errs, Options{
		Node: "node-a", Target: "PerfOptimized",
		PollInterval: time.Second, SysinfoInterval: time.Hour,
		HealthMaxStale: 5 * time.Second, ReadyzMaxFailures: 3,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    time.Now,
	})
	return p, errs, cache
}

func TestScrapeSuccessPublishesSnapshot(t *testing.T) {
	fake := &ClientFake{profile: "PerfOptimized", info: racadm.SysInfo{ServiceTag: "T1"}}
	p, _, cache := newTestPoller(fake)

	p.scrape(context.Background(), true)

	got := cache.Load()
	if !got.Success || !got.HasData {
		t.Fatalf("snapshot Success/HasData = %v/%v, want true/true", got.Success, got.HasData)
	}
	if got.Profile != "PerfOptimized" {
		t.Errorf("Profile = %q, want PerfOptimized", got.Profile)
	}
	if got.Info.ServiceTag != "T1" {
		t.Errorf("ServiceTag = %q, want T1", got.Info.ServiceTag)
	}
}

func TestScrapeFailureKeepsPreviousProfileAndCounts(t *testing.T) {
	fake := &ClientFake{profile: "PerfOptimized", info: racadm.SysInfo{ServiceTag: "T1"}}
	p, errs, cache := newTestPoller(fake)
	p.scrape(context.Background(), true) // seed good data

	fake.profileErr = &racadm.Error{Reason: "timeout", Err: errors.New("boom")}
	p.scrape(context.Background(), false)

	got := cache.Load()
	if got.Success {
		t.Error("Success = true, want false after failure")
	}
	if got.Profile != "PerfOptimized" {
		t.Errorf("Profile = %q, want previous PerfOptimized retained", got.Profile)
	}
	if c := testutil.ToFloat64(errs.WithLabelValues("node-a", "timeout")); c != 1 {
		t.Errorf("errors_total{timeout} = %v, want 1", c)
	}
}

func TestReadyAfterMaxFailures(t *testing.T) {
	fake := &ClientFake{profileErr: &racadm.Error{Reason: "exit_code", Err: errors.New("x")}}
	p, _, _ := newTestPoller(fake)

	if p.Ready() {
		t.Fatal("Ready() = true before any poll")
	}
	for i := 0; i < 3; i++ {
		p.scrape(context.Background(), false)
	}
	if !p.Ready() {
		t.Error("Ready() = false after 3 failures, want true")
	}
}

func TestReadyAfterSuccess(t *testing.T) {
	fake := &ClientFake{profile: "PerfOptimized"}
	p, _, _ := newTestPoller(fake)
	p.scrape(context.Background(), false)
	if !p.Ready() {
		t.Error("Ready() = false after success, want true")
	}
}

func TestHealthyTracksHeartbeat(t *testing.T) {
	fake := &ClientFake{profile: "PerfOptimized"}
	p, _, _ := newTestPoller(fake)
	if p.Healthy() {
		t.Fatal("Healthy() = true before first tick")
	}
	p.scrape(context.Background(), false)
	if !p.Healthy() {
		t.Error("Healthy() = false right after a tick, want true")
	}
}
