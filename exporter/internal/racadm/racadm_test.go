package racadm

import (
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
