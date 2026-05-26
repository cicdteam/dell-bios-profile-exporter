// Package racadm runs Dell racadm commands inside the host namespaces via
// nsenter and parses their output.
package racadm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
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

// Runner executes a command and returns its combined stdout. It is the single
// seam where the exporter touches the OS, so tests substitute a stub.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec, bounded by the context deadline.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Client runs racadm inside the host namespaces via nsenter.
type Client struct {
	runner      Runner
	nsenterPath string
	racadmPath  string
	timeout     time.Duration
}

// NewClient builds a Client. A zero timeout means no per-call deadline.
func NewClient(runner Runner, nsenterPath, racadmPath string, timeout time.Duration) *Client {
	return &Client{runner: runner, nsenterPath: nsenterPath, racadmPath: racadmPath, timeout: timeout}
}

func (c *Client) run(ctx context.Context, racadmArgs ...string) ([]byte, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	args := append([]string{
		"--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--",
		c.racadmPath,
	}, racadmArgs...)
	out, err := c.runner.Run(ctx, c.nsenterPath, args...)
	if err != nil {
		return nil, &Error{Reason: classify(ctx, err), Err: err}
	}
	return out, nil
}

func classify(ctx context.Context, err error) string {
	if ctx.Err() == context.DeadlineExceeded {
		return "timeout"
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return "nsenter_failed"
	}
	return "exit_code"
}

// SysProfile reads and normalizes BIOS.SysProfileSettings.SysProfile.
func (c *Client) SysProfile(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "get", "BIOS.SysProfileSettings.SysProfile")
	if err != nil {
		return "", err
	}
	return parseSysProfile(out)
}

// SysInfo reads server identity from racadm getsysinfo.
func (c *Client) SysInfo(ctx context.Context) (SysInfo, error) {
	out, err := c.run(ctx, "getsysinfo")
	if err != nil {
		return SysInfo{}, err
	}
	return parseSysInfo(out)
}
