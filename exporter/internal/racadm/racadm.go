// Package racadm runs Dell racadm commands inside the host namespaces via
// nsenter and parses their output.
package racadm

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// SysInfo holds the static server identity read from `racadm getsysinfo`.
type SysInfo struct {
	ServiceTag   string
	Model        string
	IdracVersion string
}

// Error classifies a racadm failure by reason for the errors_total metric.
// Reason is one of: timeout, exit_code, parse_error, nsenter_failed.
type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string { return fmt.Sprintf("racadm %s: %v", e.Reason, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// ReasonOf extracts the classified reason from err, defaulting to "exit_code".
func ReasonOf(err error) string {
	var rerr *Error
	if errors.As(err, &rerr) {
		return rerr.Reason
	}
	return "exit_code"
}

var (
	reSysProfile  = regexp.MustCompile(`(?m)^SysProfile=(\S+)\s*$`)
	knownProfiles = map[string]string{
		"perfoptimized":            "PerfOptimized",
		"perfperwattoptimizeddapc": "PerfPerWattOptimizedDapc",
		"perfperwattoptimizedos":   "PerfPerWattOptimizedOs",
		"densecfgoptimized":        "DenseCfgOptimized",
		"custom":                   "Custom",
	}
)

func parseSysProfile(out []byte) (string, error) {
	m := reSysProfile.FindSubmatch(out)
	if m == nil {
		return "", &Error{Reason: "parse_error", Err: errors.New("SysProfile not found in output")}
	}
	return normalizeProfile(strings.TrimSpace(string(m[1]))), nil
}

func normalizeProfile(raw string) string {
	if canonical, ok := knownProfiles[strings.ToLower(raw)]; ok {
		return canonical
	}
	return raw
}

func parseSysInfo(out []byte) (SysInfo, error) {
	var si SysInfo
	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "Service Tag":
			si.ServiceTag = val
		case "System Model":
			si.Model = val
		case "Firmware Version":
			si.IdracVersion = val
		}
	}
	return si, nil
}
