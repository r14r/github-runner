package main

import (
	"bytes"
	"context"
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
		if _, err := parseRunnerLogsCleanArgs(args); err == nil { t.Errorf("accepted %v", args) }
	}
	o, err := parseRunnerLogsCleanArgs([]string{"--repo", "owner/repo", "--days", "14", "--yes"})
	if err != nil || o.days != 14 || !o.yes { t.Fatalf("%+v %v", o, err) }
}

func TestRunnerLogsCleanPreviewAndDeletion(t *testing.T) {
	t.Setenv("RUNNER_REPO", "")
	var calls []string
	runner := diskspaceRunner{run: func(_ context.Context, name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if strings.Contains(call, "-delete") { return "", nil }
		return "/runner/_diag/Runner_old.log\n", nil
	}}
	var out bytes.Buffer
	args := []string{"--repo", "owner/repo"}
	if err := runnerLogsCleanCommand(context.Background(), args, runner, &out); err != nil { t.Fatal(err) }
	if len(calls) != 1 || strings.Contains(calls[0], "-delete") || !strings.Contains(out.String(), "DRY RUN") {
		t.Fatalf("preview unexpectedly deleted: %v %s", calls, out.String())
	}
	calls = nil
	if err := runnerLogsCleanCommand(context.Background(), append(args, "--yes"), runner, &out); err != nil { t.Fatal(err) }
	if len(calls) != 2 || !strings.Contains(calls[1], "-delete") ||
		!strings.Contains(calls[1], "/runner/_diag") ||
		!strings.Contains(calls[1], "-mmin +10080") {
		t.Fatalf("unexpected cleanup commands: %v", calls)
	}
}
