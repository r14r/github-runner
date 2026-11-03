package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestStepMonitorReportsTransitionsOnly(t *testing.T) {
	state := newMonitorState()
	var out bytes.Buffer
	job := monitorJob{ID: 123, Name: "Python, Node, Lint & Format"}
	job.Steps = append(job.Steps, struct {
		Name       string `json:"name"`
		Number     int    `json:"number"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}{Name: "Checkout repository", Number: 1, Status: "completed", Conclusion: "success"})
	state.record(job, &out)
	state.record(job, &out)
	if strings.Count(out.String(), "Checkout repository") != 1 {
		t.Fatalf("duplicate steps: %s", out.String())
	}
	job.Steps[0].Status = "in_progress"
	job.Steps[0].Conclusion = ""
	state.record(job, &out)
	if strings.Count(out.String(), "Checkout repository") != 2 {
		t.Fatalf("transition missing: %s", out.String())
	}
}
