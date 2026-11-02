package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"
)

const actionsPageSize = 100
const maxActionsPages = 1000
const cancelPollAttempts = 30

type actionsStateDescription struct {
	Name, Description string
}

// State values accepted by the GitHub Actions workflow-runs REST API.
// "all" is an explicit CLI-only filter that does not constrain status.
var actionsStates = []actionsStateDescription{
	{"all", "All workflow runs (no status filter)"},
	{"success", "Completed successfully"},
	{"failure", "Execution failed"},
	{"cancelled", "Execution cancelled"},
	{"timed_out", "Execution timed out"},
	{"action_required", "Requires manual action"},
	{"neutral", "Neutral conclusion"},
	{"skipped", "Execution skipped"},
	{"stale", "Execution became stale"},
	{"completed", "All completed runs"},
	{"queued", "Waiting in queue"},
	{"in_progress", "Currently running"},
	{"waiting", "Waiting for a condition"},
	{"pending", "Pending execution"},
	{"requested", "Execution requested"},
}

type actionsOptions struct {
	operation  string
	repo       string
	state      string
	stateSet   bool
	yes        bool
	withCancel bool
}

type workflowRun struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayTitle string `json:"display_title"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	HeadBranch  string `json:"head_branch"`
	Event       string `json:"event"`
}

type actionsAPI struct {
	list   func(repo string) ([]workflowRun, error)
	get    func(repo string, id int64) (workflowRun, error)
	cancel func(repo string, id int64) error
	delete func(repo string, id int64) error
	sleep  func(time.Duration)
}

func parseActionsArgs(args []string) (actionsOptions, error) {
	var o actionsOptions
	if len(args) == 0 {
		return o, errors.New("actions subcommand required: list, cancel or delete (use 'ghr actions --help')")
	}
	o.operation = strings.ToLower(args[0])
	switch o.operation {
	case "list", "cancel", "delete":
	default:
		return o, fmt.Errorf("unknown actions subcommand %q; expected list, cancel or delete", args[0])
	}
	repo, rest, err := requireRepo(args[1:])
	if err != nil {
		return o, err
	}
	o.repo, o.state = repo, "all"
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--state" || strings.HasPrefix(arg, "--state="):
			if o.stateSet {
				return o, errors.New("--state specified multiple times")
			}
			var value string
			if arg == "--state" {
				if i+1 >= len(rest) || strings.HasPrefix(rest[i+1], "-") {
					return o, errors.New("--state requires a value; see 'ghr actions --help'")
				}
				i++
				value = rest[i]
			} else {
				value = strings.TrimPrefix(arg, "--state=")
			}
			value = strings.ToLower(value)
			if !validActionsState(value) {
				return o, fmt.Errorf("invalid --state %q; see 'ghr actions --help' for available states", value)
			}
			o.state, o.stateSet = value, true
		case arg == "--yes":
			if o.yes {
				return o, errors.New("--yes specified multiple times")
			}
			o.yes = true
		case arg == "--with-cancel":
			if o.withCancel {
				return o, errors.New("--with-cancel specified multiple times")
			}
			o.withCancel = true
		default:
			return o, fmt.Errorf("unknown actions option %q", arg)
		}
	}
	if o.operation == "list" && o.yes {
		return o, errors.New("actions list is read-only; --yes is not applicable")
	}
	if o.operation != "delete" && o.withCancel {
		return o, errors.New("--with-cancel is only valid for actions delete")
	}
	// Prevent accidental bulk cancellation/deletion: an operator must
	// explicitly select a state (including 'all') before committing changes.
	if o.operation != "list" && o.yes && !o.stateSet {
		return o, errors.New("--yes requires explicit --state (use --state all to select all runs)")
	}
	return o, nil
}

func validActionsState(value string) bool {
	for _, candidate := range actionsStates {
		if value == candidate.Name {
			return true
		}
	}
	return false
}

func stateMatchesRun(r workflowRun, state string) bool {
	if state == "all" {
		return true
	}
	return r.Status == state || r.Conclusion == state
}

func cancellableRun(r workflowRun) bool {
	switch r.Status {
	case "queued", "in_progress", "waiting", "pending", "requested":
		return true
	default:
		return false
	}
}

// gh api is invoked without shell interpolation. The CLI must be
// authenticated; API stderr is displayed only on failure (and truncated).
func githubActionsRequest(method, endpoint string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", "--method", method, endpoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 1200 {
			detail = detail[len(detail)-1200:]
		}
		return nil, fmt.Errorf("gh api %s %s failed: %w: %s", method, endpoint, err, detail)
	}
	return output, nil
}

func newGitHubActionsAPI() actionsAPI {
	return actionsAPI{
		list: func(repo string) ([]workflowRun, error) {
			runs := []workflowRun{}
			seen := map[int64]bool{}
			for page := 1; page <= maxActionsPages; page++ {
				endpoint := fmt.Sprintf("repos/%s/actions/runs?per_page=%d&page=%d", repo, actionsPageSize, page)
				data, err := githubActionsRequest("GET", endpoint)
				if err != nil {
					return nil, fmt.Errorf("cannot retrieve workflow run page %d: %w", page, err)
				}
				var response struct {
					TotalCount   int           `json:"total_count"`
					WorkflowRuns []workflowRun `json:"workflow_runs"`
				}
				if err := json.Unmarshal(data, &response); err != nil {
					return nil, fmt.Errorf("invalid GitHub Actions run page %d: %w", page, err)
				}
				for _, item := range response.WorkflowRuns {
					if item.ID <= 0 || seen[item.ID] {
						return nil, fmt.Errorf("unexpected duplicate/invalid run ID on page %d", page)
					}
					seen[item.ID] = true
					runs = append(runs, item)
				}
				if len(response.WorkflowRuns) < actionsPageSize {
					if response.TotalCount > len(runs) {
						// GitHub may cap searchable results. Never silently
						// perform a partial bulk destructive operation.
						return nil, fmt.Errorf("GitHub returned only %d of %d workflow runs; refusing partial result", len(runs), response.TotalCount)
					}
					return runs, nil
				}
			}
			return nil, fmt.Errorf("workflow run list exceeds %d pages; refusing partial result", maxActionsPages)
		},
		get: func(repo string, id int64) (workflowRun, error) {
			data, err := githubActionsRequest("GET", fmt.Sprintf("repos/%s/actions/runs/%d", repo, id))
			if err != nil {
				return workflowRun{}, err
			}
			var result workflowRun
			if err := json.Unmarshal(data, &result); err != nil {
				return result, err
			}
			if result.ID != id {
				return result, fmt.Errorf("unexpected run ID %d when looking up %d", result.ID, id)
			}
			return result, nil
		},
		cancel: func(repo string, id int64) error {
			_, err := githubActionsRequest("POST", fmt.Sprintf("repos/%s/actions/runs/%d/cancel", repo, id))
			return err
		},
		delete: func(repo string, id int64) error {
			_, err := githubActionsRequest("DELETE", fmt.Sprintf("repos/%s/actions/runs/%d", repo, id))
			return err
		},
		sleep: time.Sleep,
	}
}

func safeTableText(value string) string {
	var result strings.Builder
	for _, r := range value {
		if r < ' ' || r == 0x7f {
			continue
		}
		result.WriteRune(r)
	}
	s := result.String()
	if len([]rune(s)) > 55 {
		return string([]rune(s)[:52]) + "..."
	}
	return s
}

func actionsCommand(args []string, api actionsAPI, output io.Writer) error {
	o, err := parseActionsArgs(args)
	if err != nil {
		return err
	}
	// Complete retrieval before any modification. IDs are snapshotted once,
	// so deletion cannot shift subsequent pagination offsets.
	all, err := api.list(o.repo)
	if err != nil {
		return err
	}
	selected := make([]workflowRun, 0, len(all))
	for _, r := range all {
		if stateMatchesRun(r, o.state) {
			selected = append(selected, r)
		}
	}
	fmt.Fprintf(output, "Repository: %s\nState: %s\nMatching workflow runs: %d\n", o.repo, o.state, len(selected))
	if o.operation == "list" {
		w := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tCONCLUSION\tWORKFLOW\tBRANCH\tTITLE")
		for _, r := range selected {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Status, r.Conclusion,
				safeTableText(r.Name), safeTableText(r.HeadBranch), safeTableText(r.DisplayTitle))
		}
		return w.Flush()
	}
	if !o.yes {
		fmt.Fprintln(output, "DRY RUN: no GitHub workflow runs will be modified. Add --yes --state STATE to execute.")
	}
	cancelCount, deleteCount, skipped := 0, 0, 0
	for _, r := range selected {
		if o.operation == "cancel" {
			if !cancellableRun(r) {
				skipped++
				continue
			}
			if !o.yes {
				fmt.Fprintf(output, "WOULD CANCEL %d (%s)\n", r.ID, r.Status)
				cancelCount++
				continue
			}
			latest, err := api.get(o.repo, r.ID)
			if err != nil {
				return fmt.Errorf("cannot verify run %d before cancellation: %w", r.ID, err)
			}
			if !cancellableRun(latest) {
				skipped++
				continue
			}
			if err := api.cancel(o.repo, r.ID); err != nil {
				return fmt.Errorf("cannot cancel run %d: %w", r.ID, err)
			}
			fmt.Fprintf(output, "CANCELLED %d\n", r.ID)
			cancelCount++
			continue
		}
		// Delete is completed-only. With --with-cancel, eligible running
		// runs are cancelled first, then checked for completion.
		if !o.withCancel && r.Status != "completed" {
			skipped++
			continue
		}
		if !o.yes {
			if r.Status == "completed" {
				fmt.Fprintf(output, "WOULD DELETE %d (%s)\n", r.ID, r.Conclusion)
				deleteCount++
			} else if o.withCancel && cancellableRun(r) {
				fmt.Fprintf(output, "WOULD CANCEL THEN DELETE %d (%s)\n", r.ID, r.Status)
				cancelCount++
				deleteCount++
			} else {
				skipped++
			}
			continue
		}
		latest, err := api.get(o.repo, r.ID)
		if err != nil {
			return fmt.Errorf("cannot verify run %d before deletion: %w", r.ID, err)
		}
		if latest.Status != "completed" {
			if !o.withCancel || !cancellableRun(latest) {
				skipped++
				continue
			}
			if err := api.cancel(o.repo, r.ID); err != nil {
				return fmt.Errorf("cannot cancel run %d before deletion: %w", r.ID, err)
			}
			fmt.Fprintf(output, "CANCEL REQUESTED %d; waiting for completion\n", r.ID)
			cancelCount++
			latest, err = awaitRunCompleted(api, o.repo, r.ID)
			if err != nil {
				return err
			}
		} else if r.Status == "completed" && !stateMatchesRun(latest, o.state) {
			// Status/conclusion changed since the initial snapshot.
			skipped++
			continue
		}
		if latest.Status != "completed" {
			return fmt.Errorf("run %d is not completed; refusing deletion", r.ID)
		}
		if err := api.delete(o.repo, r.ID); err != nil {
			return fmt.Errorf("cannot delete run %d: %w", r.ID, err)
		}
		fmt.Fprintf(output, "DELETED %d\n", r.ID)
		deleteCount++
	}
	fmt.Fprintf(output, "Result: cancel=%d delete=%d skipped=%d (dry-run=%t)\n", cancelCount, deleteCount, skipped, !o.yes)
	return nil
}

func awaitRunCompleted(api actionsAPI, repo string, id int64) (workflowRun, error) {
	for attempt := 0; attempt < cancelPollAttempts; attempt++ {
		current, err := api.get(repo, id)
		if err != nil {
			return current, fmt.Errorf("cannot check cancelled run %d: %w", id, err)
		}
		if current.Status == "completed" {
			return current, nil
		}
		if attempt+1 < cancelPollAttempts {
			api.sleep(2 * time.Second)
		}
	}
	return workflowRun{}, fmt.Errorf("run %d did not complete after cancellation; not deleted", id)
}
