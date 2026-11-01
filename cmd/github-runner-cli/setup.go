package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/r14r/github-runner/internal/workflow"
)

func commandOutput(dir string, program string, args ...string) (string, error) {
	cmd := exec.Command(program, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	b, e := cmd.Output()
	if e != nil {
		return "", fmt.Errorf("%s %s: %w", program, strings.Join(args, " "), e)
	}
	return strings.TrimSpace(string(b)), nil
}
func commandRun(dir string, program string, args ...string) error {
	cmd := exec.Command(program, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func repoSlug(repo string) (string, error) {
	repo = strings.TrimPrefix(strings.TrimPrefix(repo, "https://github.com/"), "http://github.com/")
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`).MatchString(repo) {
		return "", fmt.Errorf("invalid OWNER/REPO: %q", repo)
	}
	return strings.ToLower(strings.ReplaceAll(repo, "/", "-")), nil
}
func gitHubRepository(repo string) string {
	repo = strings.TrimPrefix(strings.TrimPrefix(repo, "https://github.com/"), "http://github.com/")
	return strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
}
func selectedBackend() string {
	b := os.Getenv("RUNNER_BACKEND")
	if b == "" || b == "auto" {
		if runtime.GOOS == "darwin" {
			// Monterey uses Docker; a native macOS runner requires >=13.5.
			v, e := commandOutput("", "sw_vers", "-productVersion")
			if e == nil {
				var major, minor int
				fmt.Sscanf(v, "%d.%d", &major, &minor)
				if major >= 14 || major == 13 && minor >= 5 {
					return "macos"
				}
			}
		}
		return "docker"
	}
	return b
}

type setupOptions struct {
	repo         string
	workflows    []string
	jobs         []string
	base         string
	allowPublic  bool
	allowCrossOS bool
	merge        bool
}

func parseSetup(args []string) (setupOptions, error) {
	var o setupOptions
	repo, rest, err := requireRepo(args)
	if err != nil {
		return o, err
	}
	o.repo = repo
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--allow-public":
			o.allowPublic = true
		case "--allow-cross-os":
			o.allowCrossOS = true
		case "--merge":
			o.merge = true
		case "--no-merge":
			o.merge = false
		case "--workflow", "--job", "--base":
			if i+1 >= len(rest) || strings.HasPrefix(rest[i+1], "--") {
				return o, fmt.Errorf("%s requires a value", rest[i])
			}
			key := rest[i]
			i++
			switch key {
			case "--workflow":
				o.workflows = append(o.workflows, rest[i])
			case "--job":
				o.jobs = append(o.jobs, rest[i])
			case "--base":
				o.base = rest[i]
			}
		default:
			return o, fmt.Errorf("unknown setup option %q", rest[i])
		}
	}
	return o, nil
}
func setupWorkflow(args []string) error {
	o, e := parseSetup(args)
	if e != nil {
		return e
	}
	slug, _ := repoSlug(o.repo)
	if e := commandRun("", "gh", "auth", "status"); e != nil {
		return fmt.Errorf("authenticate with gh auth login: %w", e)
	}
	visibility, e := commandOutput("", "gh", "repo", "view", o.repo, "--json", "visibility", "--jq", ".visibility")
	if e != nil {
		return e
	}
	if visibility == "PUBLIC" && !o.allowPublic {
		return errors.New("public repository: pass --allow-public only on an isolated runner host")
	}
	backend := selectedBackend()
	targetOS := "Linux"
	if backend == "macos" {
		targetOS = "macOS"
	} else if backend != "docker" {
		return fmt.Errorf("unsupported backend %s", backend)
	}
	if backend == "docker" {
		runner, e := newDockerRunner(o.repo, "")
		if e != nil {
			return e
		}
		if !runner.stateConfigured() {
			return fmt.Errorf("runner not registered for %s; install it first", o.repo)
		}
	} else {
		return errors.New("native macOS workflow setup is not available in the Go migration; no changes made")
	}
	if o.base == "" {
		o.base, e = commandOutput("", "gh", "repo", "view", o.repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
		if e != nil {
			return e
		}
	}
	if o.base == "" {
		return errors.New("missing base branch")
	}
	checkouts := os.Getenv("RUNNER_CHECKOUTS")
	if checkouts == "" {
		checkouts = filepath.Join(os.TempDir(), "github-runner-cli-checkouts")
	}
	if e := os.MkdirAll(checkouts, 0700); e != nil {
		return e
	}
	checkout, e := os.MkdirTemp(checkouts, slug+"-")
	if e != nil {
		return e
	}
	fmt.Printf("Checkout: %s\n", checkout)
	if e := commandRun("", "gh", "repo", "clone", o.repo, checkout, "--", "--quiet"); e != nil {
		return e
	}
	if e := commandRun(checkout, "git", "fetch", "--quiet", "origin", o.base); e != nil {
		return e
	}
	if e := commandRun(checkout, "git", "switch", "--quiet", "-C", o.base, "origin/"+o.base); e != nil {
		return e
	}
	if len(o.workflows) == 0 {
		for _, suffix := range []string{"*.yml", "*.yaml"} {
			matches, _ := filepath.Glob(filepath.Join(checkout, ".github", "workflows", suffix))
			for _, m := range matches {
				o.workflows = append(o.workflows, filepath.ToSlash(strings.TrimPrefix(m, checkout+string(os.PathSeparator))))
			}
		}
		if len(o.workflows) != 1 {
			return fmt.Errorf("found %d workflows; specify --workflow PATH for each", len(o.workflows))
		}
	}
	paths := make([]string, 0, len(o.workflows))
	for _, rel := range o.workflows {
		if !strings.HasPrefix(rel, ".github/workflows/") || strings.Contains(rel, "..") {
			return fmt.Errorf("unsafe workflow path: %q", rel)
		}
		path := filepath.Join(checkout, filepath.FromSlash(rel))
		paths = append(paths, path)
	}
	branch := fmt.Sprintf("chore/self-hosted-runner-%s-%d", time.Now().UTC().Format("20060102150405"), os.Getpid())
	if e := commandRun(checkout, "git", "switch", "--quiet", "-c", branch); e != nil {
		return e
	}
	changed, e := workflow.Patch(paths, workflow.Options{
		Label: "repo-" + slug, OS: targetOS, AllowCrossOS: o.allowCrossOS, Jobs: o.jobs,
	})
	if e != nil {
		return e
	}
	if changed == 0 {
		fmt.Println("No workflow changes required")
		return nil
	}
	if e := commandRun(checkout, "git", append([]string{"add", "--"}, o.workflows...)...); e != nil {
		return e
	}
	login, e := commandOutput("", "gh", "api", "user", "--jq", ".login")
	if e != nil {
		return e
	}
	if e := commandRun(checkout, "git", "-c", "user.name="+login, "-c", "user.email=github-actions-runner-setup@users.noreply.github.com", "commit", "-m", "ci: use self-hosted runner repo-"+slug); e != nil {
		return e
	}
	if e := commandRun(checkout, "git", "push", "--set-upstream", "origin", branch); e != nil {
		return e
	}
	pr, e := commandOutput(checkout, "gh", "pr", "create", "--repo", o.repo, "--base", o.base, "--head", branch, "--title", "ci: use self-hosted runner repo-"+slug,
		"--body", fmt.Sprintf("Route selected jobs to trusted %s x64 self-hosted runner (label repo-%s).", targetOS, slug))
	if e != nil {
		return e
	}
	fmt.Println("Pull request:", pr)
	if !o.merge {
		return nil
	}
	checks := false
	for i := 0; i < 12; i++ {
		n, err := commandOutput("", "gh", "pr", "view", pr, "--repo", o.repo, "--json", "statusCheckRollup", "--jq", ".statusCheckRollup | length")
		if err != nil {
			return err
		}
		if n != "" && n != "0" {
			checks = true
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !checks {
		return errors.New("no CI checks appeared; PR left open")
	}
	if e := commandRun("", "gh", "pr", "checks", pr, "--repo", o.repo, "--watch", "--fail-fast"); e != nil {
		return fmt.Errorf("checks failed; PR remains open: %w", e)
	}
	return commandRun("", "gh", "pr", "merge", pr, "--repo", o.repo, "--squash", "--delete-branch")
}
