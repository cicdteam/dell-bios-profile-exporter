// Package collector serves Prometheus metrics from an atomically-published
// snapshot produced by the poller.
package collector

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cicdteam/dell-bios-profile-exporter/internal/racadm"
)

// Snapshot is an immutable view of the latest poll result.
type Snapshot struct {
	Profile       string
	Info          racadm.SysInfo
	LastSuccessTS time.Time
	LastDuration  time.Duration
	Success       bool
	HasData       bool
}

// Cache holds the latest Snapshot, safe for concurrent read/write.
type Cache struct {
	ptr atomic.Pointer[Snapshot]
}

// Store publishes a new Snapshot.
func (c *Cache) Store(s Snapshot) { c.ptr.Store(&s) }

// Load returns the current Snapshot, or the zero value if none stored yet.
func (c *Cache) Load() Snapshot {
	if p := c.ptr.Load(); p != nil {
		return *p
	}
	return Snapshot{}
}

// Collector emits the snapshot-derived metrics.
type Collector struct {
	node   string
	target string
	cache  *Cache

	info          *prometheus.Desc
	matchesTarget *prometheus.Desc
	success       *prometheus.Desc
	duration      *prometheus.Desc
	lastScrape    *prometheus.Desc
}

// New builds a Collector reading from cache.
func New(node, target string, cache *Cache) *Collector {
	return &Collector{
		node:   node,
		target: target,
		cache:  cache,
		info: prometheus.NewDesc("dell_bios_sys_profile_info",
			"BIOS System Profile as a label; value is always 1.",
			[]string{"node", "profile", "service_tag", "model", "idrac_version"}, nil),
		matchesTarget: prometheus.NewDesc("dell_bios_sys_profile_matches_target",
			"Whether the current profile equals the target (1) or not (0).",
			[]string{"node", "target"}, nil),
		success: prometheus.NewDesc("dell_bios_racadm_success",
			"Whether the last racadm poll succeeded (1) or failed (0).",
			[]string{"node"}, nil),
		duration: prometheus.NewDesc("dell_bios_racadm_duration_seconds",
			"Duration of the last racadm call in seconds.",
			[]string{"node"}, nil),
		lastScrape: prometheus.NewDesc("dell_bios_last_scrape_timestamp_seconds",
			"Unix time of the last successful racadm poll.",
			[]string{"node"}, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.info
	ch <- c.matchesTarget
	ch <- c.success
	ch <- c.duration
	ch <- c.lastScrape
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	s := c.cache.Load()

	success := 0.0
	if s.Success {
		success = 1
	}
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, success, c.node)
	ch <- prometheus.MustNewConstMetric(c.duration, prometheus.GaugeValue, s.LastDuration.Seconds(), c.node)
	ch <- prometheus.MustNewConstMetric(c.lastScrape, prometheus.GaugeValue, float64(unixOrZero(s.LastSuccessTS)), c.node)

	if !s.HasData {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
		c.node, s.Profile, s.Info.ServiceTag, s.Info.Model, s.Info.IdracVersion)
	matches := 0.0
	if s.Profile == c.target {
		matches = 1
	}
	ch <- prometheus.MustNewConstMetric(c.matchesTarget, prometheus.GaugeValue, matches, c.node, c.target)
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
