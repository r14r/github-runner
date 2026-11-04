package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// wizardCommand is resumable: completed stages are detected and preserved.
// Registration, workflow changes and startup require explicit confirmation.
func wizardCommand(args []string, assets string, in io.Reader, out io.Writer) error {
	repo, rest, err := repoOption(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("wizard: unexpected arguments %v", rest)
	}
	reader := bufio.NewReader(in)
	ask := func(question string) (string, error) {
		fmt.Fprint(out, question)
		s, e := reader.ReadString('\n')
		if e != nil && e != io.EOF {
			return "", e
		}
		if e == io.EOF && strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("wizard needs interactive input")
		}
		return strings.TrimSpace(s), nil
	}
	confirm := func(question string) (bool, error) {
		s, e := ask(question + " [y/N]: ")
		if e != nil {
			return false, e
		}
		return strings.EqualFold(s, "y") || strings.EqualFold(s, "yes"), nil
	}
	if repo == "" {
		repo, err = ask("GitHub repository (OWNER/REPO): ")
		if err != nil {
			return err
		}
	}
	if _, err = repoSlug(repo); err != nil {
		return err
	}
	if selectedBackend() != "docker" {
		return fmt.Errorf("wizard requires Docker backend (RUNNER_BACKEND=docker)")
	}
	r, err := newDockerRunner(repo, assets)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[1] %-30s Repository: %s\n", "Configuration", r.Repo)
	fmt.Fprintf(out, "    %-30s Container: %s\n", "Configuration", r.Container)
	if err = r.selectPlatform(); err != nil {
		return err
	}
	step := func(number int, label, status string) {
		fmt.Fprintf(out, "[%d] %-30s %s\n", number, label, status)
	}
	imageExists := r.exists("image", r.Image)
	volumeExists := r.exists("volume", r.Volume)
	if imageExists && volumeExists {
		step(2, "Check image and volume", "Already prepared: SKIP")
	} else {
		step(2, "Check image and volume", "Image/volume missing")
		ok, e := confirm("Build image and prepare registration volume?")
		if e != nil {
			return e
		}
		if !ok {
			return nil
		}
		if err = r.download(); err != nil {
			return err
		}
	}
	if r.stateConfigured() {
		step(3, "Registration", "Existing registration: SKIP")
	} else {
		step(3, "Registration", "Not configured")
		visibility, e := commandOutput("", "gh", "repo", "view", r.Repo, "--json", "visibility", "--jq", ".visibility")
		if e != nil {
			return e
		}
		allow := false
		if strings.EqualFold(visibility, "PUBLIC") {
			allow, e = confirm("PUBLIC repository: allow executing potentially untrusted workflow code on this runner?")
			if e != nil {
				return e
			}
			if !allow {
				return fmt.Errorf("public runner requires explicit opt-in")
			}
		}
		ok, e := confirm("Register a new self-hosted runner?")
		if e != nil {
			return e
		}
		if !ok {
			return nil
		}
		if err = r.register(allow, "", true); err != nil {
			return err
		}
	}
	if r.exists("container", r.Container) {
		step(4, "Container", "Container exists: SKIP")
	} else {
		step(4, "Container", "Not installed")
		ok, e := confirm("Create runner service container?")
		if e != nil {
			return e
		}
		if !ok {
			return nil
		}
		if err = r.installContainer(); err != nil {
			return err
		}
	}
	state, e := r.docker("container", "inspect", "--format", "{{.State.Status}}", r.Container)
	if e != nil {
		return e
	}
	if state == "running" {
		step(5, "Start", "Already running: SKIP")
	} else {
		step(5, "Start", "Stopped")
		ok, e := confirm("Start the runner?")
		if e != nil {
			return e
		}
		if ok {
			if err = r.action("start"); err != nil {
				return err
			}
		}
	}
	step(6, "Workflow routing", "")
	fmt.Fprintf(out, "      Review routing with: ghr runner setup --repo %s\n", r.Repo)
	workflowDir, cleanup, checkErr := cloneDoctorWorkflowDirectory(r.Repo)
	if checkErr != nil {
		return fmt.Errorf("cannot determine workflow routing: %w", checkErr)
	}
	needsSetup, checkErr := workflowNeedsSelfHostedSetup(workflowDir)
	cleanup()
	if checkErr != nil {
		return checkErr
	}
	if !needsSetup {
		fmt.Fprintln(out, "      Workflow already configured: SKIP")
	} else {
		fmt.Fprint(out, "      ")
		ok, e := confirm("Launch workflow setup (creates a reviewable PR, does not merge automatically)?")
		if e != nil {
			return e
		}
		if ok {
			setupArgs := []string{"--repo", r.Repo, "--no-merge"}
			visibility, e := commandOutput("", "gh", "repo", "view", r.Repo, "--json", "visibility", "--jq", ".visibility")
			if e != nil {
				return e
			}
			if strings.EqualFold(visibility, "PUBLIC") {
				allowed, e := confirm("PUBLIC repository: allow routing workflows to this trusted self-hosted runner?")
				if e != nil {
					return e
				}
				if !allowed {
					fmt.Fprintln(out, "       Workflow setup skipped; no workflow changes made.")
					return nil
				}
				setupArgs = append(setupArgs, "--allow-public")
			}
			if err = setupWorkflow(setupArgs); err != nil {
				return err
			}
		}
	}
	fmt.Fprintln(out, "[7] Wizard completed.")
	fmt.Fprintf(out, "       Check: ghr status --repo %s\n", r.Repo)
	fmt.Fprintf(out, "              ghr doctor --repo %s\n", r.Repo)
	return nil
}

func runWizard(args []string, assets string) error {
	return wizardCommand(args, assets, os.Stdin, os.Stdout)
}

// workflowNeedsSelfHostedSetup detects existing self-hosted selectors without
// requiring the specific RUNNER_LABELS_JSON expression enforced by doctor.
func workflowNeedsSelfHostedSetup(root string) (bool, error) {
	files := []string{}
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		found, err := filepath.Glob(filepath.Join(root, ".github", "workflows", pattern))
		if err != nil { return false, err }
		files = append(files, found...)
	}
	if len(files) == 0 { return false, fmt.Errorf("no workflow files found") }
	jobs := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil { return false, err }
		inJobs, inJob, hasSelector, selfHosted, reusable, inRunsOnList := false, false, false, false, false, false
		flush := func() bool {
			if !inJob { return false }
			if reusable && !hasSelector { return false }
			jobs++
			return !hasSelector || !selfHosted
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSuffix(line, "\r")
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") { continue }
			if !strings.HasPrefix(line, " ") {
				if flush() { return true, nil }
				inJobs = trimmed == "jobs:"
				inJob, hasSelector, selfHosted, reusable, inRunsOnList = false, false, false, false, false
				continue
			}
			if !inJobs { continue }
			if match := doctorWorkflowJob.FindStringSubmatch(line); match != nil {
				if flush() { return true, nil }
				inJob, hasSelector, selfHosted, reusable, inRunsOnList = true, false, false, false, false
				continue
			}
			if !inJob { continue }
			if match := doctorRunsOn.FindStringSubmatch(line); match != nil {
				hasSelector, inRunsOnList = true, true
				selector := strings.ToLower(match[1])
				selfHosted = strings.Contains(selector, "self-hosted") || doctorRunnerVariable.MatchString(selector)
				continue
			}
			if doctorReusableJob.MatchString(line) { reusable = true }
			if inRunsOnList && strings.HasPrefix(trimmed, "- ") {
				if strings.Contains(strings.ToLower(trimmed), "self-hosted") { selfHosted = true }
			} else if inRunsOnList && !strings.HasPrefix(line, "      ") { inRunsOnList = false }
		}
		if flush() { return true, nil }
	}
	if jobs == 0 { return false, fmt.Errorf("no locally scheduled workflow jobs found") }
	return false, nil
}
