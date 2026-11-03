package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
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
	fmt.Fprintln(out, "GitHub Runner — Setup Wizard")
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
	fmt.Fprintf(out, "Repository: %s\nContainer: %s\n", r.Repo, r.Container)
	if err = r.selectPlatform(); err != nil {
		return err
	}
	fmt.Fprintln(out, "[1/5] Check image and volume")
	imageExists := r.exists("image", r.Image)
	volumeExists := r.exists("volume", r.Volume)
	if imageExists && volumeExists {
		fmt.Fprintln(out, "  Already prepared: SKIP")
	} else {
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
	fmt.Fprintln(out, "[2/5] Registration")
	if r.stateConfigured() {
		fmt.Fprintln(out, "  Existing registration: SKIP")
	} else {
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
	fmt.Fprintln(out, "[3/5] Container")
	if r.exists("container", r.Container) {
		fmt.Fprintln(out, "  Container exists: SKIP")
	} else {
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
	fmt.Fprintln(out, "[4/5] Start")
	state, e := r.docker("container", "inspect", "--format", "{{.State.Status}}", r.Container)
	if e != nil {
		return e
	}
	if state == "running" {
		fmt.Fprintln(out, "  Already running: SKIP")
	} else {
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
	fmt.Fprintln(out, "[5/5] Workflow routing")
	fmt.Fprintf(out, "  Review routing with: ghr runner setup --repo %s\n", r.Repo)
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
				fmt.Fprintln(out, "  Workflow setup skipped; no workflow changes made.")
				return nil
			}
			setupArgs = append(setupArgs, "--allow-public")
		}
		if err = setupWorkflow(setupArgs); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Wizard completed. Check: ghr status --repo %s; ghr doctor --repo %s\n", r.Repo, r.Repo)
	return nil
}

func runWizard(args []string, assets string) error {
	return wizardCommand(args, assets, os.Stdin, os.Stdout)
}
