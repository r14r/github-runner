package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMonitorTailValidation(t *testing.T) {
	tests := []struct {
		value string
		want  int
		ok    bool
	}{
		{"", 8, true},
		{"0", 0, true},
		{"1", 1, true},
		{"25", 25, true},
		{"1000", 1000, true},
		{"-1", 0, false},
		{"1001", 0, false},
		{"abc", 0, false},
		{"1.5", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := monitorTail(tt.value)
			if (err == nil) != tt.ok {
				t.Fatalf("monitorTail(%q) error = %v; want ok=%t", tt.value, err, tt.ok)
			}
			if tt.ok && got != tt.want {
				t.Fatalf("monitorTail(%q) = %d; want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestMonitorFollowsLogsOnceWithoutPollingOrRepeating(t *testing.T) {
	var output bytes.Buffer
	calls := []string{}
	rt := monitorRuntime{
		status: func() error {
			calls = append(calls, "status")
			return nil
		},
		runs: func() error {
			calls = append(calls, "runs")
			return nil
		},
		follow: func(tail int) error {
			calls = append(calls, "follow")
			if tail != 8 {
				t.Errorf("unexpected initial log tail: %d", tail)
			}
			return nil
		},
	}
	if err := monitorStream(&output, "", rt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"status", "runs", "follow"}) {
		t.Fatalf("monitor must read status/runs once and follow logs once: %v", calls)
	}
	for _, want := range []string{"snapshot", "last 8 lines, then new lines only", "Ctrl+C"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("monitor output missing %q: %s", want, output.String())
		}
	}
}

func TestMonitorCanStartWithOnlyNewLogLines(t *testing.T) {
	calls := 0
	var output bytes.Buffer
	rt := monitorRuntime{
		status: func() error { return nil },
		runs:   func() error { return nil },
		follow: func(tail int) error {
			calls++
			if tail != 0 {
				t.Fatalf("expected 0 existing lines, got %d", tail)
			}
			return nil
		},
	}
	if err := monitorStream(&output, "0", rt); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected one long-lived stream, got %d", calls)
	}
}

func TestMonitorInvalidTailDoesNotCallDockerOrGitHub(t *testing.T) {
	called := false
	rt := monitorRuntime{
		status: func() error { called = true; return nil },
		runs:   func() error { called = true; return nil },
		follow: func(int) error { called = true; return nil },
	}
	var output bytes.Buffer
	if err := monitorStream(&output, "not-a-number", rt); err == nil {
		t.Fatal("invalid tail accepted")
	}
	if called || output.Len() != 0 {
		t.Fatalf("invalid environment value triggered operations or output")
	}
}

func TestMonitorInvalidTailFailsBeforeDockerPreflight(t *testing.T) {
	t.Setenv("RUNNER_MONITOR_TAIL", "invalid")
	r := dockerRunner{Repo: "example-org/demo-app", Container: "nonexistent"}
	if err := r.action("monitor"); err == nil || !strings.Contains(err.Error(), "RUNNER_MONITOR_TAIL") {
		t.Fatalf("expected monitor configuration error before Docker preflight: %v", err)
	}
}

func TestMonitorStatusErrorsWarnButStreamingContinues(t *testing.T) {
	var output bytes.Buffer
	joined := []string{}
	rt := monitorRuntime{
		status: func() error {
			joined = append(joined, "status")
			return errors.New("cannot reach status API")
		},
		runs: func() error {
			joined = append(joined, "runs")
			return errors.New("GitHub API unavailable")
		},
		follow: func(tail int) error {
			joined = append(joined, "follow")
			return nil
		},
	}
	if err := monitorStream(&output, "4", rt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(joined, []string{"status", "runs", "follow"}) {
		t.Fatal(joined)
	}
	if strings.Count(output.String(), "WARNING:") != 2 {
		t.Fatalf("expected two warnings, got %q", output.String())
	}
}

func TestMonitorFollowErrorReturnedToCaller(t *testing.T) {
	expected := errors.New("docker daemon disconnected")
	rt := monitorRuntime{
		status: func() error { return nil },
		runs:   func() error { return nil },
		follow: func(int) error { return expected },
	}
	if err := monitorStream(&bytes.Buffer{}, "0", rt); !errors.Is(err, expected) {
		t.Fatalf("lost log stream error: %v", err)
	}
}

func TestDockerMonitorUsesFollowAndTailInsteadOfRepeatedLogs(t *testing.T) {
	want := []string{"logs", "--follow", "--tail", "8", "github-runner-example"}
	if got := dockerMonitorLogArgs("github-runner-example", 8); !reflect.DeepEqual(got, want) {
		t.Fatalf("actual Docker arguments: %v; want %v", got, want)
	}
}
