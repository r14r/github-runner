package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type monitorRun struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}
type monitorJob struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Steps      []struct {
		Name       string `json:"name"`
		Number     int    `json:"number"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"steps"`
}
type monitorState struct {
	seen        map[string]string
	initialized bool
	known       map[int64]bool
}

func newMonitorState() *monitorState {
	return &monitorState{seen: map[string]string{}, known: map[int64]bool{}}
}
func (s *monitorState) record(job monitorJob, out io.Writer) {
	for _, step := range job.Steps {
		if step.Status == "" || step.Status == "pending" {
			continue
		}
		key := fmt.Sprintf("%d/%d", job.ID, step.Number)
		value := step.Status + "/" + step.Conclusion
		if s.seen[key] == value {
			continue
		}
		s.seen[key] = value
		fmt.Fprintf(out, "[step] %s | %02d %s — %s", job.Name, step.Number, step.Name, step.Status)
		if step.Conclusion != "" {
			fmt.Fprintf(out, " (%s)", step.Conclusion)
		}
		fmt.Fprintln(out)
	}
}

// pollWorkflowSteps prints transitions, not repeated snapshots. GitHub supplies
// step state through the Jobs REST API, not through the runner container logs.
func (s *monitorState) poll(repo string, out io.Writer) error {
	raw, err := commandOutput("", "gh", "api", "repos/"+repo+"/actions/runs?per_page=10")
	if err != nil {
		return err
	}
	var runs struct {
		WorkflowRuns []monitorRun `json:"workflow_runs"`
	}
	if err = json.Unmarshal([]byte(raw), &runs); err != nil {
		return err
	}
	if !s.initialized {
		// Ignore runs already completed before monitor was attached.
		for _, r := range runs.WorkflowRuns {
			if r.Status == "completed" {
				s.known[r.ID] = true
			}
		}
		s.initialized = true
	}
	for _, run := range runs.WorkflowRuns {
		if run.Status == "completed" && s.known[run.ID] {
			continue
		}
		raw, err := commandOutput("", "gh", "api", fmt.Sprintf("repos/%s/actions/runs/%d/jobs?per_page=100", repo, run.ID))
		if err != nil {
			return err
		}
		var response struct {
			Jobs []monitorJob `json:"jobs"`
		}
		if err = json.Unmarshal([]byte(raw), &response); err != nil {
			return err
		}
		for _, job := range response.Jobs {
			s.record(job, out)
		}
		if run.Status == "completed" {
			s.known[run.ID] = true
		}
	}
	return nil
}

func followRunnerAndSteps(repo, container, tailValue string, out io.Writer) error {
	tail, err := monitorTail(tailValue)
	if err != nil {
		return err
	}
	if strings.TrimSpace(container) == "" {
		return fmt.Errorf("runner container name is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI required for step monitoring: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	docker := exec.CommandContext(ctx, "docker", "logs", "--follow", "--tail", fmt.Sprint(tail), container)
	docker.Stdout = out
	docker.Stderr = os.Stderr
	if err := docker.Start(); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() { result <- docker.Wait() }()
	state := newMonitorState()
	// Poll immediately and then periodically; step transitions are based on
	// GitHub's view and can be delayed by API publication.
	if err := state.poll(repo, out); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: step monitoring unavailable: %v\n", err)
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			return err
		case <-ticker.C:
			if err := state.poll(repo, out); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: step monitoring: %v\n", err)
			}
		}
	}
}
