package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("NODE_NAME", "node-a")
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		PollInterval:      60 * time.Second,
		SysinfoInterval:   3600 * time.Second,
		RequestTimeout:    15 * time.Second,
		RacadmPath:        "/opt/dell/srvadmin/sbin/racadm",
		NsenterPath:       "/usr/bin/nsenter",
		NodeName:          "node-a",
		LogLevel:          slog.LevelInfo,
		ListenPort:        9101,
		TargetProfile:     "PerfOptimized",
		ReadyzMaxFailures: 3,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("POLL_INTERVAL", "30")
	t.Setenv("LISTEN_PORT", "8080")
	t.Setenv("LOG_LEVEL", "DEBUG")
	t.Setenv("TARGET_PROFILE", "DenseCfgOptimized")
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want 30s", got.PollInterval)
	}
	if got.ListenPort != 8080 {
		t.Errorf("ListenPort = %d, want 8080", got.ListenPort)
	}
	if got.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want DEBUG", got.LogLevel)
	}
	if got.TargetProfile != "DenseCfgOptimized" {
		t.Errorf("TargetProfile = %q, want DenseCfgOptimized", got.TargetProfile)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("LISTEN_PORT", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for invalid LISTEN_PORT")
	}
}
