// Package config loads exporter configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the resolved exporter configuration.
type Config struct {
	PollInterval      time.Duration
	SysinfoInterval   time.Duration
	RequestTimeout    time.Duration
	RacadmPath        string
	NsenterPath       string
	NodeName          string
	LogLevel          slog.Level
	ListenPort        int
	TargetProfile     string
	ReadyzMaxFailures int
}

// Load reads configuration from the environment, applying defaults. It returns
// an error if a numeric variable is set but cannot be parsed, so that
// misconfiguration fails fast instead of being silently ignored.
func Load() (Config, error) {
	c := Config{
		RacadmPath:    envString("RACADM_PATH", "/opt/dell/srvadmin/sbin/racadm"),
		NsenterPath:   envString("NSENTER_PATH", "/usr/bin/nsenter"),
		NodeName:      envString("NODE_NAME", ""),
		TargetProfile: envString("TARGET_PROFILE", "PerfOptimized"),
	}

	var err error
	if c.PollInterval, err = envSeconds("POLL_INTERVAL", 60); err != nil {
		return Config{}, err
	}
	if c.SysinfoInterval, err = envSeconds("SYSINFO_INTERVAL", 3600); err != nil {
		return Config{}, err
	}
	if c.RequestTimeout, err = envSeconds("REQUEST_TIMEOUT", 15); err != nil {
		return Config{}, err
	}
	if c.ListenPort, err = envInt("LISTEN_PORT", 9101); err != nil {
		return Config{}, err
	}
	if c.ReadyzMaxFailures, err = envInt("READYZ_MAX_FAILURES", 3); err != nil {
		return Config{}, err
	}
	c.LogLevel = parseLevel(envString("LOG_LEVEL", "INFO"))
	return c, nil
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return n, nil
}

func envSeconds(key string, def int) (time.Duration, error) {
	n, err := envInt(key, def)
	if err != nil {
		return 0, err
	}
	return time.Duration(n) * time.Second, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARNING", "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
