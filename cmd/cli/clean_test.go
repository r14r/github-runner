package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunnerLogsCleanArguments(t *testing.T) {
	t.Setenv("RUNNER_REPO", "")
	for _, args := range [][]string{
		nil,
		{"--repo", "invalid"},
		{"--repo", "owner/repo", "--days", "0"},
		{"--repo", "owner/repo", "--days", "bad"},
		{"--repo", "owner/repo", "--days"},
		{"--repo", "owner/repo", "--bogus"},
	} {
		if _, err := parseRunnerLogsCleanArgs(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	o, err := parseRunnerLogsCleanArgs([]string{"--repo", "owner/repo", "--days", "14", "--yes"})
	if err != nil || o.days != 14 || !o.yes {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestRunnerLogsCleanPreviewAndDeletion(t *testing.T) {
	t.Setenv("RUNNER_REPO", "")
	var calls []string
	runner := diskspaceRunner{run: func(_ context.Context, name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if strings.Contains(call, "inspect --format") {
			return "running\\n", nil
		}
		if strings.Contains(call, "-delete") {
			return "", nil
		}
		return "/runner/_diag/Runner_old.log\n", nil
	}}
	var out bytes.Buffer
	args := []string{"--repo", "owner/repo"}
	if err := runnerLogsCleanCommand(context.Background(), args, runner, &out); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.Contains(strings.Join(calls, " "), "-delete") || !strings.Contains(out.String(), "DRY RUN") {
		t.Fatalf("preview unexpectedly deleted: %v %s", calls, out.String())
	}
	calls = nil
	if err := runnerLogsCleanCommand(context.Background(), append(args, "--yes"), runner, &out); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || !strings.Contains(calls[2], "-delete") ||
		!strings.Contains(calls[2], "/runner/_diag") ||
		!strings.Contains(calls[2], "-mmin +10080") {
		t.Fatalf("unexpected cleanup commands: %v", calls)
	}
}

func TestRunnerLogsCleanStoppedContainerHint(t *testing.T) {
	t.Setenv("RUNNER_REPO", "")
	calls := 0
	runner := diskspaceRunner{run: func(_ context.Context, name string, args ...string) (string, error) {
		calls++
		if !strings.Contains(strings.Join(args, " "), "inspect --format") {
			t.Fatal("must not execute find in stopped container")
		}
		return "exited\n", nil
	}}
	err := runnerLogsCleanCommand(context.Background(), []string{"--repo", "owner/repo"}, runner, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not running") ||
		!strings.Contains(err.Error(), "ghr start --repo owner/repo") || calls != 1 {
		t.Fatalf("expected actionable stopped-container hint, got %v, calls=%d", err, calls)
	}
}

func TestRunnerLogsCleanMissingContainerHint(t *testing.T) {
	t.Setenv("RUNNER_REPO", "")
	runner := diskspaceRunner{run: func(_ context.Context, name string, args ...string) (string, error) {
		return "", errors.New("No such object")
	}}
	err := runnerLogsCleanCommand(context.Background(), []string{"--repo", "owner/repo"}, runner, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "docker ps -a") {
		t.Fatalf("expected missing-container hint, got %v", err)
	}
}
