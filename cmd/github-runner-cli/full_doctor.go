package main

import (
	"bytes"
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

// doctorBadge uses a fixed visible width so the third column remains aligned
// even when ANSI background colors are enabled.
func doctorBadge(ok, color bool) string {
	label := "  OK  "
	if !ok {
		label = " FAIL "
	}
	if !color {
		return label
	}
	if ok {
		return "\x1b[30;42m" + label + "\x1b[0m"
	}
	return "\x1b[97;41m" + label + "\x1b[0m"
}

func doctorColorEnabled(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func doctorFields(details string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(details, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return fields
}

var doctorWorkflowSummary = regexp.MustCompile(`\b([0-9]+)/([0-9]+) jobs use variable\b`)

func doctorStageDescription(number int, title, details string) string {
	fields := doctorFields(details)
	switch number {
	case 1:
		if fields["Container"] != "" {
			return fmt.Sprintf("Docker: %s (%s; image matches)", fields["Container"], fields["State"])
		}
	case 2:
		// Reuse the existing toolchain checks but show only their summary.
		for _, line := range strings.Split(details, "\n") {
			if summary, ok := strings.CutPrefix(strings.TrimSpace(line), "RESULT: PASS — "); ok {
				return "Toolchain: " + summary + "; " + fields["Workflows"] + " workflows"
			}
		}
	case 3:
		if fields["Name"] != "" {
			return fmt.Sprintf("Registration: %s (ID %s; volume %s)", fields["Name"], fields["ID"], fields["Volume"])
		}
	case 4:
		if fields["Name"] != "" {
			return fmt.Sprintf("GitHub: %s (%s; busy=%s; labels match)", fields["Name"], fields["Status"], fields["Busy"])
		}
	case 5:
		files, matched, total := 0, 0, 0
		for _, line := range strings.Split(details, "\n") {
			if match := doctorWorkflowSummary.FindStringSubmatch(line); match != nil {
				var okCount, jobCount int
				_, _ = fmt.Sscanf(match[1], "%d", &okCount)
				_, _ = fmt.Sscanf(match[2], "%d", &jobCount)
				files++
				matched += okCount
				total += jobCount
			}
		}
		if files > 0 {
			return fmt.Sprintf("Workflows: %d files, %d/%d jobs use RUNNER_LABELS_JSON", files, matched, total)
		}
	case 6:
		if fields["RUNNER_LABELS_JSON"] != "" {
			return "RUNNER_LABELS_JSON: " + fields["RUNNER_LABELS_JSON"]
		}
	}
	return title
}

type doctorFailure struct{ causes []error }

func (e *doctorFailure) Error() string { return errors.Join(e.causes...).Error() }
func (e *doctorFailure) Unwrap() error { return errors.Join(e.causes...) }

func runFullDoctor(out io.Writer, repo string, checks doctorChecks) error {
	return runFullDoctorWithColor(out, repo, checks, doctorColorEnabled(out))
}

// No banner or nested diagnostic output: each stage is one three-column row.
func runFullDoctorWithColor(out io.Writer, repo string, checks doctorChecks, color bool) error {
	failures := []error{}
	check := func(number int, title string, work func(io.Writer) error) {
		var details bytes.Buffer
		err := work(&details)
		description := doctorStageDescription(number, title, details.String())
		if err != nil {
			description += " — " + err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", title, err))
		}
		fmt.Fprintf(out, "%02d  %s  %s\n", number, doctorBadge(err == nil, color), description)
	}
	var identity runnerIdentity
	var identityErr error
	check(1, "Docker container", checks.container)
	check(2, "Image and toolchain", checks.toolchain)
	check(3, "Local runner registration", func(out io.Writer) error {
		identity, identityErr = checks.registration(out)
		return identityErr
	})
	check(4, "GitHub runner status and labels", func(out io.Writer) error {
		if identityErr != nil {
			return fmt.Errorf("local registration not verified; %w", identityErr)
		}
		return checks.github(out, identity)
	})
	check(5, "Workflow runner selectors", checks.workflows)
	check(6, "RUNNER_LABELS_JSON repository variable", checks.variable)
	if len(failures) > 0 {
		fmt.Fprintf(out, "--  %s  RESULT: FAIL — %d of 6 checks failed (%s)\n",
			doctorBadge(false, color), len(failures), repo)
		return &doctorFailure{causes: failures}
	}
	fmt.Fprintf(out, "--  %s  RESULT: PASS — all 6 checks OK (%s)\n", doctorBadge(true, color), repo)
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
			if err != nil {
				return err
			}
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
			return toolchainDoctorOutput(r, out)
		},
		registration: func(out io.Writer) (runnerIdentity, error) {
			local, err := inspectLocal(r.Slug)
			if err != nil {
				return runnerIdentity{}, err
			}
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
			if err != nil {
				return err
			}
			record, err := matchedRegistration(id, registered)
			if err != nil {
				return err
			}
			if record == nil {
				return fmt.Errorf("%s (ID %d) not registered with GitHub", id.AgentName, id.AgentID)
			}
			fmt.Fprintf(out, "    Name:   %s\n    Status: %s\n    Busy:   %t\n", record.Name, record.Status, record.Busy)
			if err := verifyGitHubDoctorRunner(*record, r.Slug); err != nil {
				return err
			}
			return nil
		},
		workflows: func(out io.Writer) error {
			dir, cleanup, err := cloneDoctorWorkflowDirectory(r.Repo)
			if err != nil {
				return err
			}
			defer cleanup()
			return checkDoctorWorkflowDirectory(dir, out)
		},
		variable: func(out io.Writer) error {
			data, err := commandOutput("", "gh", "api", "repos/"+r.Repo+"/actions/variables/RUNNER_LABELS_JSON")
			if err != nil {
				return fmt.Errorf("RUNNER_LABELS_JSON missing or inaccessible; set it with 'gh variable set RUNNER_LABELS_JSON --repo %s --body ...': %w", r.Repo, err)
			}
			var response struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal([]byte(data), &response); err != nil {
				return fmt.Errorf("invalid GitHub variable API response: %w", err)
			}
			if err := validateDoctorRunnerVariable(response.Value, r.Slug); err != nil {
				return err
			}
			fmt.Fprintf(out, "    RUNNER_LABELS_JSON: %s\n", response.Value)
			return nil
		},
	})
}

func verifyGitHubDoctorRunner(record runnerRecord, slug string) error {
	if record.Status != "online" {
		return fmt.Errorf("GitHub runner is %s, expected online", record.Status)
	}
	if record.Busy {
		return errors.New("GitHub runner is busy (not idle); retry after the current job")
	}
	required := map[string]bool{
		"self-hosted": false, "linux": false, "x64": false, "repo-" + strings.ToLower(slug): false,
	}
	for _, label := range record.Labels {
		key := strings.ToLower(label.Name)
		if _, exists := required[key]; exists {
			required[key] = true
		}
	}
	missing := []string{}
	for label, found := range required {
		if !found {
			missing = append(missing, label)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("GitHub runner is missing labels: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validateDoctorRunnerVariable(value, slug string) error {
	var labels []string
	if err := json.Unmarshal([]byte(value), &labels); err != nil {
		return fmt.Errorf("RUNNER_LABELS_JSON must contain a JSON array of runner labels: %w", err)
	}
	required := map[string]bool{"self-hosted": false, "linux": false, "x64": false, "repo-" + strings.ToLower(slug): false}
	for _, label := range labels {
		key := strings.ToLower(strings.TrimSpace(label))
		if _, found := required[key]; found {
			required[key] = true
		}
	}
	missing := []string{}
	for label, found := range required {
		if !found {
			missing = append(missing, label)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("RUNNER_LABELS_JSON lacks required labels: %s", strings.Join(missing, ", "))
	}
	return nil
}

var doctorWorkflowJob = regexp.MustCompile(`^  ([A-Za-z0-9_.-]+):\s*(?:#.*)?$`)
var doctorRunsOn = regexp.MustCompile(`^    runs-on:\s*(.*?)\s*$`)
var doctorReusableJob = regexp.MustCompile(`^    uses:\s*\S+`)
var doctorRunnerVariable = regexp.MustCompile(`(?i)fromJSON\s*\(\s*vars\.RUNNER_LABELS_JSON\b`)

type doctorWorkflowResult struct {
	filename                 string
	count, checked, reusable int
	problems                 []string
}

func checkDoctorWorkflowSource(name string, data []byte) doctorWorkflowResult {
	result := doctorWorkflowResult{filename: name}
	inJobs := false
	job := ""
	runsOn := ""
	reusable := false
	flush := func() {
		if job == "" {
			return
		}
		result.count++
		switch {
		case strings.Contains(runsOn, "${{") && strings.Contains(runsOn, "}}") && doctorRunnerVariable.MatchString(runsOn):
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
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			flush()
			job = ""
			runsOn = ""
			reusable = false
			inJobs = trimmed == "jobs:"
			continue
		}
		if !inJobs {
			continue
		}
		if match := doctorWorkflowJob.FindStringSubmatch(line); match != nil {
			flush()
			job, runsOn, reusable = match[1], "", false
			continue
		}
		if job != "" {
			if match := doctorRunsOn.FindStringSubmatch(line); match != nil {
				runsOn = strings.TrimSpace(strings.SplitN(match[1], " #", 2)[0])
			}
			if doctorReusableJob.MatchString(line) {
				reusable = true
			}
		}
	}
	flush()
	if result.count == 0 {
		result.problems = append(result.problems, name+": no job definitions detected")
	}
	return result
}

func checkDoctorWorkflowDirectory(root string, out io.Writer) error {
	dir := filepath.Join(root, ".github", "workflows")
	files := []string{}
	for _, extension := range []string{"*.yaml", "*.yml"} {
		found, err := filepath.Glob(filepath.Join(dir, extension))
		if err != nil {
			return err
		}
		files = append(files, found...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return errors.New("no .github/workflows/*.yaml or *.yml found")
	}
	failures := []error{}
	jobs := 0
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !info.Mode().IsRegular() {
			failures = append(failures, fmt.Errorf("%s is not a regular workflow file", path))
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		r := checkDoctorWorkflowSource(filepath.Base(path), data)
		jobs += r.count - r.reusable
		fmt.Fprintf(out, "    %-30s %d/%d jobs use variable", r.filename, r.checked, r.count-r.reusable)
		if r.reusable > 0 {
			fmt.Fprintf(out, " (%d reusable job(s) exempt)", r.reusable)
		}
		fmt.Fprintln(out)
		for _, item := range r.problems {
			failures = append(failures, errors.New(item))
		}
	}
	if jobs == 0 {
		failures = append(failures, errors.New("no locally scheduled workflow jobs found"))
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}

func cloneDoctorWorkflowDirectory(repo string) (string, func(), error) {
	if _, err := os.Stat(os.TempDir()); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "ghr-doctor-workflows-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := capturedGit("", "gh", "repo", "clone", repo, dir, "--", "--depth=1", "--filter=blob:none", "--sparse", "--quiet"); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("cannot clone %s: %w", repo, err)
	}
	if err := capturedGit(dir, "git", "sparse-checkout", "set", ".github/workflows"); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("cannot check out workflows: %w", err)
	}
	return dir, cleanup, nil
}
