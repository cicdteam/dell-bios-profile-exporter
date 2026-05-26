package racadm

import (
	"context"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseSysProfile(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "standard",
			in:   "[Key=BIOS.Setup.1-1#SysProfileSettings]\nSysProfile=PerfOptimized\n",
			want: "PerfOptimized",
		},
		{
			name: "lowercase normalized",
			in:   "SysProfile=perfoptimized\n",
			want: "PerfOptimized",
		},
		{
			name: "trailing spaces",
			in:   "SysProfile=DenseCfgOptimized   \n",
			want: "DenseCfgOptimized",
		},
		{
			name:    "missing",
			in:      "[Key=...]\nSomethingElse=1\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSysProfile([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseSysProfile err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("parseSysProfile = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeProfile(t *testing.T) {
	tests := map[string]string{
		"perfoptimized":            "PerfOptimized",
		"PerfOptimized":            "PerfOptimized",
		"PERFPERWATTOPTIMIZEDDAPC": "PerfPerWattOptimizedDapc",
		"perfperwattoptimizedos":   "PerfPerWattOptimizedOs",
		"densecfgoptimized":        "DenseCfgOptimized",
		"custom":                   "Custom",
		"SomethingUnknown":         "SomethingUnknown",
	}
	for in, want := range tests {
		if got := normalizeProfile(in); got != want {
			t.Errorf("normalizeProfile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSysInfo(t *testing.T) {
	in := `RAC Information:
Service Tag         = ABCD1234
System Model        = PowerEdge R650
Firmware Version    = 7.10.30.00
`
	want := SysInfo{ServiceTag: "ABCD1234", Model: "PowerEdge R650", IdracVersion: "7.10.30.00"}
	got, err := parseSysInfo([]byte(in))
	if err != nil {
		t.Fatalf("parseSysInfo error = %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("parseSysInfo mismatch (-want +got):\n%s", diff)
	}
}

// RunnerStub returns canned output and records the last command it was asked
// to run.
type RunnerStub struct {
	out     []byte
	err     error
	gotName string
	gotArgs []string
}

func (s *RunnerStub) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.gotName = name
	s.gotArgs = args
	return s.out, s.err
}

func TestClientSysProfileBuildsNsenterCommand(t *testing.T) {
	stub := &RunnerStub{out: []byte("SysProfile=PerfOptimized\n")}
	c := NewClient(stub, "/usr/bin/nsenter", "/opt/dell/srvadmin/sbin/racadm", 0)

	got, err := c.SysProfile(context.Background())
	if err != nil {
		t.Fatalf("SysProfile error = %v", err)
	}
	if got != "PerfOptimized" {
		t.Errorf("SysProfile = %q, want PerfOptimized", got)
	}
	if stub.gotName != "/usr/bin/nsenter" {
		t.Errorf("runner name = %q, want /usr/bin/nsenter", stub.gotName)
	}
	wantArgs := []string{
		"--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--",
		"/opt/dell/srvadmin/sbin/racadm", "get", "BIOS.SysProfileSettings.SysProfile",
	}
	if diff := cmp.Diff(wantArgs, stub.gotArgs); diff != "" {
		t.Errorf("runner args mismatch (-want +got):\n%s", diff)
	}
}

func TestClientSysProfileClassifiesExitError(t *testing.T) {
	stub := &RunnerStub{err: &exec.ExitError{}}
	c := NewClient(stub, "/usr/bin/nsenter", "/racadm", 0)
	_, err := c.SysProfile(context.Background())
	if ReasonOf(err) != "exit_code" {
		t.Errorf("ReasonOf = %q, want exit_code", ReasonOf(err))
	}
}

func TestClientSysProfileClassifiesNsenterMissing(t *testing.T) {
	stub := &RunnerStub{err: &exec.Error{Name: "nsenter", Err: exec.ErrNotFound}}
	c := NewClient(stub, "/usr/bin/nsenter", "/racadm", 0)
	_, err := c.SysProfile(context.Background())
	if ReasonOf(err) != "nsenter_failed" {
		t.Errorf("ReasonOf = %q, want nsenter_failed", ReasonOf(err))
	}
}

func TestClientSysInfo(t *testing.T) {
	stub := &RunnerStub{out: []byte("Service Tag = XYZ\nSystem Model = PowerEdge R750\nFirmware Version = 6.0\n")}
	c := NewClient(stub, "/usr/bin/nsenter", "/racadm", 0)
	got, err := c.SysInfo(context.Background())
	if err != nil {
		t.Fatalf("SysInfo error = %v", err)
	}
	want := SysInfo{ServiceTag: "XYZ", Model: "PowerEdge R750", IdracVersion: "6.0"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SysInfo mismatch (-want +got):\n%s", diff)
	}
	wantArgs := []string{"--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "/racadm", "getsysinfo"}
	if diff := cmp.Diff(wantArgs, stub.gotArgs); diff != "" {
		t.Errorf("SysInfo args mismatch (-want +got):\n%s", diff)
	}
}
