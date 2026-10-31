package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var version = "dev"

var hostActions = map[string]string{
	"install": "install.sh", "migrate": "migrate-macos.sh", "repair": "repair-macos.sh",
	"doctor": "doctor.sh", "start": "control.sh", "stop": "control.sh",
	"status": "control.sh", "monitor": "control.sh", "actions-cancel": "actions.sh",
	"actions-delete": "actions.sh", "actions-clear": "actions.sh", "cleanup": "cleanup.sh",
}

func assetsDir(executable, cwd string) (string, error) {
	if override := os.Getenv("GITHUB_RUNNER_CLI_ENV"); override != "" {
		if s, e := os.Stat(override); e == nil && s.IsDir() {
			return override, nil
		}
		return "", fmt.Errorf("GITHUB_RUNNER_CLI_ENV not found: %s", override)
	}
	for _, p := range []string{filepath.Join(filepath.Dir(executable), "env", "github-runner-cli"), cwd} {
		if s, e := os.Stat(filepath.Join(p, "runner", "docker", "Dockerfile")); e == nil && !s.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("runtime assets missing: run 'just install' or set GITHUB_RUNNER_CLI_ENV")
}
func execute(script string, args []string) error {
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func usage() {
	fmt.Print(`github-runner-cli — Go GitHub Actions runner manager

Usage:
  github-runner-cli version
  github-runner-cli install OWNER/REPO [--allow-public] [--name NAME]
  github-runner-cli start OWNER/REPO
  github-runner-cli stop OWNER/REPO
  github-runner-cli status OWNER/REPO
  github-runner-cli monitor OWNER/REPO
  github-runner-cli refresh OWNER/REPO
  github-runner-cli runner docker-doctor
  github-runner-cli runner <download|register|setup|pr|install-service|start|stop|status|monitor|replace-container> OWNER/REPO [options]
  github-runner-cli host <install|doctor|start|stop|status|monitor|migrate|repair|cleanup|actions-cancel|actions-delete|actions-clear> [args]

Environment:
  RUNNER_BACKEND=docker|macos|auto (auto selects Docker on macOS Monterey)
  RUNNER_PROFILE=auto|generic|bookbuilder (BookBuilder selects required toolchain)
  RUNNER_DOCKER_SOCKET=0|1 (privileged Docker socket requires explicit opt-in)
  GITHUB_RUNNER_CLI_ENV=<installation assets directory>

The Go CLI uses native Docker/GitHub workflow orchestration. Legacy host service
scripts remain isolated in the installation assets pending host-specific migration.
`)
}
func parseRegisterFlags(args []string) (bool, string, error) {
	allowPublic := false
	name := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--allow-public":
			allowPublic = true
		case "--name":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return false, "", errors.New("--name requires a value")
			}
			i++
			name = args[i]
		default:
			return false, "", fmt.Errorf("unknown option: %s", args[i])
		}
	}
	return allowPublic, name, nil
}
func run(args []string, executable, cwd string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "version":
		fmt.Println("github-runner-cli " + version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	}
	env, err := assetsDir(executable, cwd)
	if err != nil {
		return err
	}
	command := strings.ToLower(args[0])
	if command == "install" {
		if len(args) < 2 {
			return errors.New("usage: install OWNER/REPO [--allow-public]")
		}
		allow, name, e := parseRegisterFlags(args[2:])
		if e != nil {
			return e
		}
		runner, e := newDockerRunner(args[1], env)
		if e != nil {
			return e
		}
		if selectedBackend() == "macos" {
			return errors.New("native macOS runner setup not migrated: use the explicitly retained runner scripts")
		}
		return runner.install(allow, name)
	}
	if command == "refresh" || command == "start" || command == "stop" || command == "status" || command == "monitor" {
		if len(args) != 2 {
			return fmt.Errorf("usage: %s OWNER/REPO", command)
		}
		runner, e := newDockerRunner(args[1], env)
		if e != nil {
			return e
		}
		if selectedBackend() == "macos" {
			action := command
			if action == "refresh" {
				action = "replace-container"
			}
			return execute(filepath.Join(env, "runner", "04-run.sh"), []string{args[1], action})
		}
		return runner.action(command)
	}
	switch command {
	case "runner":
		if len(args) < 2 {
			return errors.New("runner action required")
		}
		action := strings.ToLower(args[1])
		if action == "docker-doctor" {
			if len(args) != 2 {
				return errors.New("docker-doctor accepts no arguments")
			}
			return dockerDoctor()
		}
		if action == "setup" || action == "pr" {
			return setupWorkflow(args[2:])
		}
		if len(args) < 3 {
			return errors.New("runner action requires OWNER/REPO")
		}
		runner, e := newDockerRunner(args[2], env)
		if e != nil {
			return e
		}
		if selectedBackend() == "macos" {
			file := "04-run.sh"
			switch action {
			case "download":
				file = "01-download.sh"
			case "register":
				file = "02-configure.sh"
			}
			return execute(filepath.Join(env, "runner", file), args[2:])
		}
		switch action {
		case "download":
			if len(args) != 3 {
				return errors.New("download accepts OWNER/REPO only")
			}
			return runner.download()
		case "register", "configure":
			allow, name, e := parseRegisterFlags(args[3:])
			if e != nil {
				return e
			}
			return runner.register(allow, name, false)
		case "install-service", "start", "stop", "status", "monitor", "foreground", "replace-container", "uninstall-service":
			if len(args) != 3 {
				return fmt.Errorf("%s accepts OWNER/REPO only", action)
			}
			return runner.action(action)
		default:
			return fmt.Errorf("unknown runner action: %s", action)
		}
	case "host":
		if len(args) < 2 {
			return errors.New("host action required")
		}
		action := strings.ToLower(args[1])
		script, ok := hostActions[action]
		if !ok {
			return fmt.Errorf("unknown host action: %s", action)
		}
		rest := append([]string(nil), args[2:]...)
		if strings.HasPrefix(action, "actions-") {
			rest = append([]string{strings.TrimPrefix(action, "actions-")}, rest...)
		}
		if script == "control.sh" {
			rest = append([]string{action}, rest...)
		}
		if action == "cleanup" && len(rest) == 0 {
			rest = []string{"30"}
		}
		return execute(filepath.Join(env, script), rest)
	default:
		return fmt.Errorf("unknown command %q (try --help)", args[0])
	}
}
func main() {
	cwd, e := os.Getwd()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	exe, e := os.Executable()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if e := run(os.Args[1:], exe, cwd); e != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", e)
		var exit *exec.ExitError
		if errors.As(e, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
