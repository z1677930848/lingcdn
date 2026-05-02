package server

import (
	"reflect"
	"testing"
)

// TestBuildUpgradeCommand pins down the wrapping choices for each privilege
// mode: root runs bash directly, sudo mode prepends `sudo -n -E`, and
// "none" falls through to bash (so the script can surface a clear error
// instead of us silently aborting).
func TestBuildUpgradeCommand(t *testing.T) {
	const shellCmd = `curl -fsSL 'https://x' | bash -s -- --channel 'stable'`

	cases := []struct {
		name      string
		mode      string
		wantBin   string
		wantArgs  []string
	}{
		{
			name:     "root runs bash directly",
			mode:     "root",
			wantBin:  "/bin/bash",
			wantArgs: []string{"-c", shellCmd},
		},
		{
			name:     "sudo wraps with -n -E",
			mode:     "sudo",
			wantBin:  "sudo",
			wantArgs: []string{"-n", "-E", "/bin/bash", "-c", shellCmd},
		},
		{
			name:     "none falls through to bash",
			mode:     "none",
			wantBin:  "/bin/bash",
			wantArgs: []string{"-c", shellCmd},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, args := buildUpgradeCommand(privilegeEscalation{mode: tc.mode}, shellCmd)
			if bin != tc.wantBin {
				t.Fatalf("binary: got %q want %q", bin, tc.wantBin)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Fatalf("args:\ngot  %#v\nwant %#v", args, tc.wantArgs)
			}
		})
	}
}

// TestChoosePrivilegeEscalationHostSmoke is a loose smoke-test of the host
// probe — it doesn't pin a specific mode (varies by CI/dev machine) but
// ensures the function always returns one of the three known modes.
func TestChoosePrivilegeEscalationHostSmoke(t *testing.T) {
	got := choosePrivilegeEscalation()
	switch got.mode {
	case "root", "sudo", "none":
		return
	default:
		t.Fatalf("unexpected mode %q", got.mode)
	}
}
