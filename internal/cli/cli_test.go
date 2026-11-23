package cli

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func run(t *testing.T, info BuildInfo, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Execute(info, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionPrintsLdflagsValues(t *testing.T) {
	info := BuildInfo{Version: "v0.1.0", Commit: "abc1234", Date: "2026-11-23T10:00:00Z"}
	code, out, errOut := run(t, info, "version")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	want := "pixopher v0.1.0 (commit abc1234, built 2026-11-23T10:00:00Z)\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	code, _, errOut := run(t, BuildInfo{}, "version", "extra")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (stderr %q)", code, errOut)
	}
}

func TestResolve(t *testing.T) {
	vcs := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.1-0.20261123100000-abcdef123456"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.time", Value: "2026-11-23T10:00:00Z"},
		},
	}
	devel := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	tests := []struct {
		name string
		in   BuildInfo
		bi   *debug.BuildInfo
		want BuildInfo
	}{
		{
			name: "no build info",
			in:   BuildInfo{},
			want: BuildInfo{DefaultVersion, DefaultCommit, DefaultDate},
		},
		{
			name: "go install fills from build info",
			in:   BuildInfo{DefaultVersion, DefaultCommit, DefaultDate},
			bi:   vcs,
			want: BuildInfo{"v0.1.1-0.20261123100000-abcdef123456", "abcdef1234567890", "2026-11-23T10:00:00Z"},
		},
		{
			name: "ldflags win over build info",
			in:   BuildInfo{"v0.1.0", "1111111", "2026-01-01T00:00:00Z"},
			bi:   vcs,
			want: BuildInfo{"v0.1.0", "1111111", "2026-01-01T00:00:00Z"},
		},
		{
			name: "devel version stays dev",
			in:   BuildInfo{},
			bi:   devel,
			want: BuildInfo{DefaultVersion, DefaultCommit, DefaultDate},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Resolve(func() (*debug.BuildInfo, bool) { return tt.bi, tt.bi != nil })
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			code, out, errOut := run(t, BuildInfo{}, "completion", shell)
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			if !strings.Contains(out, "pixopher") {
				t.Fatalf("completion script does not mention pixopher:\n%s", out)
			}
		})
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	code, _, errOut := run(t, BuildInfo{}, "nope")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "unknown command") {
		t.Fatalf("stderr %q does not mention the unknown command", errOut)
	}
}
