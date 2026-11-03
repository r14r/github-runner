package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultMonitorTail = 10
const maxMonitorTail = 1000

// monitorTail controls the number of historical lines printed once on attach.
// A value of zero means that only lines emitted after attachment are shown.
func monitorTail(value string) (int, error) {
	if value == "" {
		return defaultMonitorTail, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > maxMonitorTail {
		return 0, fmt.Errorf("RUNNER_MONITOR_TAIL must be an integer from 0 to %d", maxMonitorTail)
	}
	return n, nil
}

// followRunnerLogs is intentionally equivalent to "docker logs -f --tail N".
// In particular it must not query GitHub, poll status, emit headings or replay
// recent logs on a timer. Docker writes directly to the terminal as lines arrive.
func followRunnerLogs(container, tailValue string, run func(string, ...string) error) error {
	tail, err := monitorTail(tailValue)
	if err != nil {
		return err
	}
	if strings.TrimSpace(container) == "" {
		return fmt.Errorf("runner container name is required to stream logs")
	}
	return run("docker", "logs", "--follow", "--tail", strconv.Itoa(tail), container)
}

func (r dockerRunner) monitor() error {
	return followRunnerAndSteps(r.Repo, r.Container, os.Getenv("RUNNER_MONITOR_TAIL"), os.Stdout)
}
