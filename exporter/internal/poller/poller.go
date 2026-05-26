// Package poller periodically runs racadm and publishes the result to the cache.
package poller

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cicdteam/dell-bios-profile-exporter/internal/collector"
	"github.com/cicdteam/dell-bios-profile-exporter/internal/racadm"
)

// ProfileClient is the racadm surface the poller depends on.
type ProfileClient interface {
	SysProfile(ctx context.Context) (string, error)
	SysInfo(ctx context.Context) (racadm.SysInfo, error)
}

// Options configures a Poller.
type Options struct {
	Node              string
	Target            string
	PollInterval      time.Duration
	SysinfoInterval   time.Duration
	HealthMaxStale    time.Duration
	ReadyzMaxFailures int
	Logger            *slog.Logger
	Now               func() time.Time // defaults to time.Now
}

// Poller runs the background scrape loop.
type Poller struct {
	client ProfileClient
	cache  *collector.Cache
	errors *prometheus.CounterVec
	opts   Options
	now    func() time.Time

	lastTick  atomic.Int64 // unix nanos of the most recent loop tick
	failCount atomic.Int64 // consecutive failures
}

// New builds a Poller.
func New(client ProfileClient, cache *collector.Cache, errs *prometheus.CounterVec, opts Options) *Poller {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Poller{client: client, cache: cache, errors: errs, opts: opts, now: now}
}

// Run scrapes immediately, then on every PollInterval tick until ctx is done.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.opts.PollInterval)
	defer ticker.Stop()

	p.scrape(ctx, true)
	lastSysinfo := p.now()
	for {
		select {
		case <-ctx.Done():
			p.opts.Logger.Info("poller stopping")
			return
		case <-ticker.C:
			fetchInfo := p.now().Sub(lastSysinfo) >= p.opts.SysinfoInterval
			p.scrape(ctx, fetchInfo)
			if fetchInfo {
				lastSysinfo = p.now()
			}
		}
	}
}

func (p *Poller) scrape(ctx context.Context, fetchInfo bool) {
	p.lastTick.Store(p.now().UnixNano())
	prev := p.cache.Load()

	start := p.now()
	profile, err := p.client.SysProfile(ctx)
	dur := p.now().Sub(start)
	if err != nil {
		reason := racadm.ReasonOf(err)
		p.errors.WithLabelValues(p.opts.Node, reason).Inc()
		p.failCount.Add(1)
		next := prev
		next.Success = false
		next.LastDuration = dur
		p.cache.Store(next)
		p.opts.Logger.Warn("scrape failed", "reason", reason, "duration_ms", dur.Milliseconds())
		return
	}
	p.failCount.Store(0)

	info := prev.Info
	if fetchInfo || !prev.HasData {
		if si, serr := p.client.SysInfo(ctx); serr == nil {
			info = si
		} else {
			reason := racadm.ReasonOf(serr)
			p.errors.WithLabelValues(p.opts.Node, reason).Inc()
			p.opts.Logger.Warn("sysinfo failed", "reason", reason)
		}
	}

	p.cache.Store(collector.Snapshot{
		Profile:       profile,
		Info:          info,
		LastSuccessTS: p.now(),
		LastDuration:  dur,
		Success:       true,
		HasData:       true,
	})
	p.opts.Logger.Info("scrape ok", "profile", profile, "duration_ms", dur.Milliseconds())
	if profile != p.opts.Target {
		p.opts.Logger.Warn("drift detected", "profile", profile, "target", p.opts.Target)
	}
}

// Healthy reports whether the loop has ticked recently. Independent of racadm
// success.
func (p *Poller) Healthy() bool {
	last := p.lastTick.Load()
	if last == 0 {
		return false
	}
	return p.now().Sub(time.Unix(0, last)) < p.opts.HealthMaxStale
}

// Ready reports true after the first successful poll, or after
// ReadyzMaxFailures consecutive failures so readiness never blocks forever.
func (p *Poller) Ready() bool {
	return p.cache.Load().HasData || int(p.failCount.Load()) >= p.opts.ReadyzMaxFailures
}
