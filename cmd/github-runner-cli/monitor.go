package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

const defaultMonitorTail = 8
const maxMonitorTail = 1000

// monitorRuntime keeps stream lifecycle testable without running Docker or GH.
type monitorRuntime struct {
	status func() error
	runs   func() error
	follow func(tail int) error
}

func monitorTail(value string) (int, error) {
	if value == "" {
		return defaultMonitorTail, nil
	}
	tail, err := strconv.Atoi(value)
	if err != nil || tail < 0 || tail > maxMonitorTail {
		return 0, fmt.Errorf("RUNNER_MONITOR_TAIL must be an integer between 0 and %d", maxMonitorTail)
	}
	return tail, nil
}

// monitorStream prints status and recent runs once, then blocks on the live
// Docker log follower. No timer, polling loop, or replay of older log lines.
func monitorStream(out io.Writer, value string, rt monitorRuntime) error {
	tail, err := monitorTail(value)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "--- Runner status (snapshot) ---")
	if err := rt.status(); err != nil {
		fmt.Fprintf(out, "WARNING: runner status unavailable: %v\n", err)
	}
	fmt.Fprintln(out, "--- Recent GitHub Actions runs (snapshot) ---")
	if err := rt.runs(); err != nil {
		fmt.Fprintf(out, "WARNING: recent run list unavailable: %v\n", err)
	}
	fmt.Fprintf(out, "--- Streaming runner logs (last %d lines, then new lines only) ---\n", tail)
	fmt.Fprintln(out, "Press Ctrl+C to stop monitoring; the runner keeps running.")
	return rt.follow(tail)
}

func dockerMonitorLogArgs(container string, tail int) []string {
	return []string{"logs", "--follow", "--tail", strconv.Itoa(tail), container}
}

func (r dockerRunner) monitor() error {
	if !r.exists("container", r.Container) {
		return fmt.Errorf("runner container %s is missing; install and start it first", r.Container)
	}
	return monitorStream(os.Stdout, os.Getenv("RUNNER_MONITOR_TAIL"), monitorRuntime{
		status: func() error {
			return r.action("status")
		},
		runs: func() error {
			return commandRun("", "gh", "run", "list", "--repo", r.Repo, "--limit", "5")
		},
		follow: func(tail int) error {
			return commandRun("", "docker", dockerMonitorLogArgs(r.Container, tail)...)
		},
	})
}
