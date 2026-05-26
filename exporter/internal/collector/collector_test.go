package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cicdteam/dell-bios-profile-exporter/internal/racadm"
)

func TestCollectWithData(t *testing.T) {
	cache := &Cache{}
	cache.Store(Snapshot{
		Profile:       "PerfOptimized",
		Info:          racadm.SysInfo{ServiceTag: "ABCD1234", Model: "PowerEdge R650", IdracVersion: "7.10.30.00"},
		LastSuccessTS: time.Unix(1700000000, 0),
		LastDuration:  1500 * time.Millisecond,
		Success:       true,
		HasData:       true,
	})
	c := New("node-a", "PerfOptimized", cache)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	want := `
# HELP dell_bios_last_scrape_timestamp_seconds Unix time of the last successful racadm poll.
# TYPE dell_bios_last_scrape_timestamp_seconds gauge
dell_bios_last_scrape_timestamp_seconds{node="node-a"} 1.7e+09
# HELP dell_bios_racadm_duration_seconds Duration of the last racadm call in seconds.
# TYPE dell_bios_racadm_duration_seconds gauge
dell_bios_racadm_duration_seconds{node="node-a"} 1.5
# HELP dell_bios_racadm_success Whether the last racadm poll succeeded (1) or failed (0).
# TYPE dell_bios_racadm_success gauge
dell_bios_racadm_success{node="node-a"} 1
# HELP dell_bios_sys_profile_info BIOS System Profile as a label; value is always 1.
# TYPE dell_bios_sys_profile_info gauge
dell_bios_sys_profile_info{idrac_version="7.10.30.00",model="PowerEdge R650",node="node-a",profile="PerfOptimized",service_tag="ABCD1234"} 1
# HELP dell_bios_sys_profile_matches_target Whether the current profile equals the target (1) or not (0).
# TYPE dell_bios_sys_profile_matches_target gauge
dell_bios_sys_profile_matches_target{node="node-a",target="PerfOptimized"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

func TestCollectMatchesTargetZeroOnDrift(t *testing.T) {
	cache := &Cache{}
	cache.Store(Snapshot{Profile: "Custom", Success: true, HasData: true})
	c := New("node-a", "PerfOptimized", cache)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	if got := toMatchesTarget(t, reg); got != 0 {
		t.Errorf("matches_target = %v, want 0", got)
	}
}

func TestCollectNoDataEmitsSuccessZero(t *testing.T) {
	cache := &Cache{}
	c := New("node-a", "PerfOptimized", cache)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	want := `
# HELP dell_bios_last_scrape_timestamp_seconds Unix time of the last successful racadm poll.
# TYPE dell_bios_last_scrape_timestamp_seconds gauge
dell_bios_last_scrape_timestamp_seconds{node="node-a"} 0
# HELP dell_bios_racadm_duration_seconds Duration of the last racadm call in seconds.
# TYPE dell_bios_racadm_duration_seconds gauge
dell_bios_racadm_duration_seconds{node="node-a"} 0
# HELP dell_bios_racadm_success Whether the last racadm poll succeeded (1) or failed (0).
# TYPE dell_bios_racadm_success gauge
dell_bios_racadm_success{node="node-a"} 0
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"dell_bios_racadm_success",
		"dell_bios_racadm_duration_seconds",
		"dell_bios_last_scrape_timestamp_seconds",
	); err != nil {
		t.Error(err)
	}
	// info + matches_target must be absent without data.
	if err := testutil.GatherAndCompare(reg, strings.NewReader(""),
		"dell_bios_sys_profile_info", "dell_bios_sys_profile_matches_target"); err != nil {
		t.Error(err)
	}
}

// toMatchesTarget gathers the registry and returns the value of the single
// dell_bios_sys_profile_matches_target series.
func toMatchesTarget(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "dell_bios_sys_profile_matches_target" {
			return mf.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatal("matches_target not found")
	return 0
}
