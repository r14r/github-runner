package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMonitorTailDefaultsToTailFBehavior(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
		valid bool
	}{
		{"", 10, true},
		{"0", 0, true},
		{"1", 1, true},
		{"30", 30, true},
		{"1000", 1000, true},
		{"-1", 0, false},
		{"1001", 0, false},
		{"garbage", 0, false},
		{"1.5", 0, false},
	} {
		got, err := monitorTail(tc.value)
		if (err == nil) != tc.valid {
			t.Errorf("monitorTail(%q): %v (expected valid=%v)", tc.value, err, tc.valid)
		}
		if tc.valid && got != tc.want {
			t.Errorf("monitorTail(%q) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestMonitorRunsOnlyOneContinuousDockerFollow(t *testing.T) {
	for _, tc := range []struct{ tail, want string }{
		{"", "10"},
		{"0", "0"},
		{"25", "25"},
	} {
		var calls [][]string
		err := followRunnerLogs("github-runner-example-org-demo", tc.tail, func(program string, args ...string) error {
			calls = append(calls, append([]string{program}, args...))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		want := [][]string{{"docker", "logs", "--follow", "--tail", tc.want, "github-runner-example-org-demo"}}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("tail=%q calls=%q want=%q", tc.tail, calls, want)
		}
	}
}

func TestMonitorDoesNotPollOrQueryGitHubAndDoesNotEmitHeaders(t *testing.T) {
	// The command passed to Docker is the entire monitoring behavior.
	// The actual terminal streams come straight from commandRun.
	got := 0
	err := followRunnerLogs("runner", "", func(program string, args ...string) error {
		got++
		for _, a := range append([]string{program}, args...) {
			for _, forbidden := range []string{"gh", "run", "list", "status", "inspect", "sleep"} {
				if a == forbidden {
					t.Fatalf("unexpected status/polling operation %q", a)
				}
			}
		}
		return nil
	})
	if err != nil || got != 1 {
		t.Fatalf("expected exactly one Docker follow: count=%d err=%v", got, err)
	}
}

func TestMonitorRejectsBadTailWithoutRunningDocker(t *testing.T) {
	called := false
	err := followRunnerLogs("runner", "nope", func(string, ...string) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "RUNNER_MONITOR_TAIL") || called {
		t.Fatalf("invalid tail must fail before running Docker: called=%v err=%v", called, err)
	}
}

func TestMonitorEmptyContainerRejected(t *testing.T) {
	called := false
	err := followRunnerLogs("", "10", func(string, ...string) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("missing runner container should fail: called=%v err=%v", called, err)
	}
}

func TestMonitorPropagatesDockerError(t *testing.T) {
	expected := errors.New("Docker daemon disconnected")
	err := followRunnerLogs("runner", "0", func(string, ...string) error {
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("Docker error was swallowed: %v", err)
	}
}

func TestMonitorInvalidConfigIsCheckedBeforePlatformAndRegistration(t *testing.T) {
	t.Setenv("RUNNER_MONITOR_TAIL", "invalid")
	r := dockerRunner{Repo: "example-org/demo", Container: "missing-container"}
	err := r.action("monitor")
	if err == nil || !strings.Contains(err.Error(), "RUNNER_MONITOR_TAIL") {
		t.Fatalf("expected config failure, not Docker/registration failure: %v", err)
	}
}
