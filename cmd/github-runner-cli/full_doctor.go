package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// fullDoctor is intentionally read-only: it never registers or refreshes
// runners, writes workflow files or changes repository variables.
type doctorChecks struct {
	container    func(io.Writer) error
	toolchain    func(io.Writer) error
	registration func(io.Writer) (runnerIdentity, error)
	github       func(io.Writer, runnerIdentity) error
	workflows    func(io.Writer) error
	variable     func(io.Writer) error
}

func runFullDoctor(out io.Writer, repo string, checks doctorChecks) error {
	fmt.Fprintf(out, "GitHub Runner — Full Doctor\nRepository: %s\n", repo)
	failures := []error{}
	check := func(number int, title string, work func() error) {
		fmt.Fprintf(out, "\n%02d: %s\n", number, title)
		if err := work(); err != nil {
			fmt.Fprintf(out, "    Status: FAIL — %s\n", err)
			failures = append(failures, fmt.Errorf("%s: %w", title, err))
		} else {
			fmt.Fprintln(out, "    Status: OK")
		}
	}
	var identity runnerIdentity
	var identityErr error
	check(1, "Check local Docker container", func() error { return checks.container(out) })
	check(2, "Check image and toolchain", func() error { return checks.toolchain(out) })
	check(3, "Check local runner registration", func() error {
		identity, identityErr = checks.registration(out)
		return identityErr
	})
	check(4, "Check GitHub runner status and labels", func() error {
		if identityErr != nil {
			return fmt.Errorf("local registration not verified; %w", identityErr)
		}
		return checks.github(out, identity)
	})
	check(5, "Check all workflow runner selectors", func() error { return checks.workflows(out) })
	check(6, "Check RUNNER_LABELS_JSON repository variable", func() error { return checks.variable(out) })
	if len(failures) > 0 {
		fmt.Fprintf(out, "\nRESULT: FAIL — %d of 6 checks failed\n", len(failures))
		return errors.Join(failures...)
	}
	fmt.Fprintln(out, "\nRESULT: PASS — all 6 checks OK")
	return nil
}

func fullDoctor(r dockerRunner) error {
	return runFullDoctor(os.Stdout, r.Repo, doctorChecks{
		container: func(out io.Writer) error {
			if err := r.selectPlatform(); err != nil {
				return err
			}
			if !r.exists("container", r.Container) {
				return fmt.Errorf("%s not installed; run ghr install --repo %s", r.Container, r.Repo)
			}
			state, err := r.docker("container", "inspect", "--format", "{{.State.Status}}", r.Container)
			if err != nil { return err }
			fmt.Fprintf(out, "    Container: %s\n    State:     %s\n", r.Container, state)
			if state != "running" {
				return fmt.Errorf("container is %s; run ghr start --repo %s", state, r.Repo)
			}
			if !r.imageMatches() {
				return fmt.Errorf("container image differs from %s; rebuild then ghr refresh --repo %s", r.Image, r.Repo)
			}
			fmt.Fprintf(out, "    Image:     %s (matches running container)\n", r.Image)
			return nil
		},
		toolchain: func(out io.Writer) error {
			// Reuse the existing image-only doctor so 'ghr build' still works
			// before any runner is installed or registered.
			return toolchainDoctor(r)
		},
		registration: func(out io.Writer) (runnerIdentity, error) {
			local, err := inspectLocal(r.Slug)
			if err != nil { return runnerIdentity{}, err }
			if !existingVolume(r.Volume) {
				return runnerIdentity{}, fmt.Errorf("registration volume %s not found", r.Volume)
			}
			if local.Repository == "" || !strings.EqualFold(local.Repository, r.Repo) {
				return runnerIdentity{}, fmt.Errorf("local runner belongs to %q, expected %s", local.Repository, r.Repo)
			}
			if local.Identity.AgentID <= 0 || local.Identity.AgentName == "" {
				return runnerIdentity{}, errors.New("runner identity missing agentId or agentName in registration volume")
			}
			fmt.Fprintf(out, "    Volume: %s\n    Name:   %s\n    ID:     %d\n", r.Volume, local.Identity.AgentName, local.Identity.AgentID)
			return local.Identity, nil
		},
		github: func(out io.Writer, id runnerIdentity) error {
			registered, err := githubRunners(r.Repo)
			if err != nil { return err }
			record, err := matchedRegistration(id, registered)
			if err != nil { return err }
			if record == nil { return fmt.Errorf("%s (ID %d) not registered with GitHub", id.AgentName, id.AgentID) }
			fmt.Fprintf(out, "    Name:   %s\n    Status: %s\n    Busy:   %t\n", record.Name, record.Status, record.Busy)
			if err := verifyGitHubDoctorRunner(*record, r.Slug); err != nil { return err }
			return nil
		},
		workflows: func(out io.Writer) error {
			dir, cleanup, err := cloneDoctorWorkflowDirectory(r.Repo)
			if err != nil { return err }
			defer cleanup()
			return checkDoctorWorkflowDirectory(dir, out)
		},
		variable: func(out io.Writer) error {
			data, err := commandOutput("", "gh", "api", "repos/"+r.Repo+"/actions/variables/RUNNER_LABELS_JSON")
			if err != nil { return fmt.Errorf("RUNNER_LABELS_JSON missing or inaccessible; set it with 'gh variable set RUNNER_LABELS_JSON --repo %s --body ...': %w", r.Repo, err) }
			var response struct { Value string `json:"value"` }
			if err := json.Unmarshal([]byte(data), &response); err != nil { return fmt.Errorf("invalid GitHub variable API response: %w", err) }
			if err := validateDoctorRunnerVariable(response.Value, r.Slug); err != nil { return err }
			fmt.Fprintf(out, "    RUNNER_LABELS_JSON: %s\n", response.Value)
			return nil
		},
	})
}

func verifyGitHubDoctorRunner(record runnerRecord, slug string) error {
	if record.Status != "online" { return fmt.Errorf("GitHub runner is %s, expected online", record.Status) }
	if record.Busy { return errors.New("GitHub runner is busy (not idle); retry after the current job") }
	required := map[string]bool{
		"self-hosted": false, "linux": false, "x64": false, "repo-"+strings.ToLower(slug): false,
	}
	for _, label := range record.Labels {
		key := strings.ToLower(label.Name)
		if _, exists := required[key]; exists { required[key] = true }
	}
	missing := []string{}
	for label, found := range required { if !found { missing = append(missing, label) } }
	sort.Strings(missing)
	if len(missing) > 0 { return fmt.Errorf("GitHub runner is missing labels: %s", strings.Join(missing, ", ")) }
	return nil
}

func validateDoctorRunnerVariable(value, slug string) error {
	var labels []string
	if err := json.Unmarshal([]byte(value), &labels); err != nil {
		return fmt.Errorf("RUNNER_LABELS_JSON must contain a JSON array of runner labels: %w", err)
	}
	required := map[string]bool{"self-hosted": false, "linux": false, "x64": false, "repo-"+strings.ToLower(slug): false}
	for _, label := range labels {
		key := strings.ToLower(strings.TrimSpace(label))
		if _, found := required[key]; found { required[key] = true }
	}
	missing := []string{}
	for label, found := range required { if !found { missing = append(missing, label) } }
	sort.Strings(missing)
	if len(missing) > 0 { return fmt.Errorf("RUNNER_LABELS_JSON lacks required labels: %s", strings.Join(missing, ", ")) }
	return nil
}

var doctorWorkflowJob = regexp.MustCompile(`^  ([A-Za-z0-9_.-]+):\s*(?:#.*)?$`)
var doctorRunsOn = regexp.MustCompile(`^    runs-on:\s*(.*?)\s*$`)
var doctorReusableJob = regexp.MustCompile(`^    uses:\s*\S+`)
var doctorRunnerVariable = regexp.MustCompile(`(?i)fromJSON\s*\(\s*vars\.RUNNER_LABELS_JSON\b`)

type doctorWorkflowResult struct { filename string; count, checked, reusable int; problems []string }

func checkDoctorWorkflowSource(name string, data []byte) doctorWorkflowResult {
	result := doctorWorkflowResult{filename: name}
	inJobs := false
	job := ""
	runsOn := ""
	reusable := false
	flush := func() {
		if job == "" { return }
		result.count++
		switch {
		case doctorRunnerVariable.MatchString(runsOn):
			result.checked++
		case reusable && runsOn == "":
			result.reusable++
		case runsOn == "":
			result.problems = append(result.problems, fmt.Sprintf("%s: job %s missing runs-on", name, job))
		default:
			result.problems = append(result.problems, fmt.Sprintf("%s: job %s does not use fromJSON(vars.RUNNER_LABELS_JSON)", name, job))
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") { continue }
		if !strings.HasPrefix(line, " ") {
			flush(); job = ""; runsOn = ""; reusable = false
			inJobs = trimmed == "jobs:"
			continue
		}
		if !inJobs { continue }
		if match := doctorWorkflowJob.FindStringSubmatch(line); match != nil {
			flush(); job, runsOn, reusable = match[1], "", false
			continue
		}
		if job != "" {
			if match := doctorRunsOn.FindStringSubmatch(line); match != nil {
				runsOn = strings.TrimSpace(strings.SplitN(match[1], " #", 2)[0])
			}
			if doctorReusableJob.MatchString(line) { reusable = true }
		}
	}
	flush()
	if result.count == 0 { result.problems = append(result.problems, name+": no job definitions detected") }
	return result
}

func checkDoctorWorkflowDirectory(root string, out io.Writer) error {
	dir := filepath.Join(root, ".github", "workflows")
	files := []string{}
	for _, extension := range []string{"*.yaml", "*.yml"} {
		found, err := filepath.Glob(filepath.Join(dir, extension))
		if err != nil { return err }
		files = append(files, found...)
	}
	sort.Strings(files)
	if len(files) == 0 { return errors.New("no .github/workflows/*.yaml or *.yml found") }
	failures := []error{}
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil { failures = append(failures, err); continue }
		if !info.Mode().IsRegular() { failures = append(failures, fmt.Errorf("%s is not a regular workflow file", path)); continue }
		data, err := os.ReadFile(path)
		if err != nil { failures = append(failures, err); continue }
		r := checkDoctorWorkflowSource(filepath.Base(path), data)
		fmt.Fprintf(out, "    %-30s %d/%d jobs use variable", r.filename, r.checked, r.count-r.reusable)
		if r.reusable > 0 { fmt.Fprintf(out, " (%d reusable job(s) exempt)", r.reusable) }
		fmt.Fprintln(out)
		for _, item := range r.problems { failures = append(failures, errors.New(item)) }
	}
	if len(failures) > 0 { return errors.Join(failures...) }
	return nil
}

func cloneDoctorWorkflowDirectory(repo string) (string, func(), error) {
	if _, err := os.Stat(os.TempDir()); err != nil { return "", nil, err }
	dir, err := os.MkdirTemp("", "ghr-doctor-workflows-")
	if err != nil { return "", nil, err }
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := capturedGit("", "gh", "repo", "clone", repo, dir, "--", "--depth=1", "--filter=blob:none", "--sparse", "--quiet"); err != nil {
		cleanup(); return "", nil, fmt.Errorf("cannot clone %s: %w", repo, err)
	}
	if err := capturedGit(dir, "git", "sparse-checkout", "set", ".github/workflows"); err != nil {
		cleanup(); return "", nil, fmt.Errorf("cannot check out workflows: %w", err)
	}
	return dir, cleanup, nil
}
