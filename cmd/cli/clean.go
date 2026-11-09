package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// runnerLogsCleanOptions targets only old diagnostics. The default retention
// protects logs of current or recently completed jobs.
type runnerLogsCleanOptions struct {
	repo string
	days int
	yes  bool
}

func parseRunnerLogsCleanArgs(args []string) (runnerLogsCleanOptions, error) {
	o := runnerLogsCleanOptions{days: 7}
	repo, rest, err := requireRepo(args)
	if err != nil {
		return o, err
	}
	o.repo = repo
	daysSet := false
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--yes":
			if o.yes {
				return o, errors.New("--yes specified multiple times")
			}
			o.yes = true
		case "--days":
			if daysSet || i+1 == len(rest) {
				return o, errors.New("--days requires one integer value")
			}
			i++
			n, err := strconv.Atoi(rest[i])
			if err != nil || n < 1 || n > 3650 {
				return o, errors.New("--days must be between 1 and 3650")
			}
			o.days = n
			daysSet = true
		default:
			return o, fmt.Errorf("unknown clean option %q", rest[i])
		}
	}
	return o, nil
}

func runnerLogsCleanCommand(ctx context.Context, args []string, runner diskspaceRunner, out io.Writer) error {
	o, err := parseRunnerLogsCleanArgs(args)
	if err != nil {
		return err
	}
	r, err := newDockerRunner(o.repo, "")
	if err != nil {
		return err
	}
	// Only enumerate exact runner diagnostics (not workspaces, credentials,
	// volumes or arbitrary files). 'find' is executed directly, not in a shell.
	// -mtime +N is rounded to full days, so use -mmin for precise retention.
	minutes := strconv.Itoa(o.days * 24 * 60)
	findArgs := []string{
		"exec", "--user", "0", r.Container, "find", "/runner/_diag",
		"-type", "f", "(", "-name", "Runner_*.log", "-o", "-name", "Worker_*.log", ")",
		"-mmin", "+" + minutes,
	}
	files, err := runner.run(ctx, "docker", append(append([]string{}, findArgs...), "-print")...)
	if err != nil {
		return fmt.Errorf("cannot inspect runner logs in %s (container must be running): %w", r.Container, err)
	}
	fmt.Fprintf(out, "Runner: %s\nRetention: %d days\n", o.repo, o.days)
	trimmed := strings.TrimSpace(files)
	if trimmed == "" {
		fmt.Fprintln(out, "No matching old runner diagnostic logs.")
		return nil
	}
	fmt.Fprintln(out, "Matching logs:")
	fmt.Fprintln(out, trimmed)
	if !o.yes {
		fmt.Fprintln(out, "DRY RUN: no files removed. Use --yes to delete these old diagnostic logs.")
		return nil
	}
	// Re-evaluate retention when deleting, so a concurrent run cannot cause
	// deletion of a file that no longer satisfies the selection.
	_, err = runner.run(ctx, "docker", append(append([]string{}, findArgs...), "-delete")...)
	if err != nil {
		return fmt.Errorf("runner diagnostics cleanup failed: %w", err)
	}
	fmt.Fprintln(out, "Old runner diagnostic logs removed.")
	return nil
}
