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
        if s, e := os.Stat(override); e == nil && s.IsDir() { return override, nil }
        return "", fmt.Errorf("GITHUB_RUNNER_CLI_ENV not found: %s", override)
    }
    for _, p := range []string{filepath.Join(filepath.Dir(executable), "env", "github-runner-cli"), cwd} {
        if s, e := os.Stat(filepath.Join(p, "runner", "docker", "Dockerfile")); e == nil && !s.IsDir() { return p, nil }
    }
    return "", errors.New("runtime assets missing: run 'just install' or set GITHUB_RUNNER_CLI_ENV")
}

func execute(script string, args []string, repo string) error {
    cmd := exec.Command("bash", append([]string{script}, args...)...)
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    if repo != "" { cmd.Env = append(os.Environ(), "RUNNER_REPO="+repo) }
    return cmd.Run()
}

func usage() {
    fmt.Print(`github-runner-cli — GitHub Actions runner manager (Go)

Usage:
  github-runner-cli version
  github-runner-cli install --repo OWNER/REPO [--allow-public] [--name NAME]
  github-runner-cli start --repo OWNER/REPO
  github-runner-cli stop --repo OWNER/REPO
  github-runner-cli status --repo OWNER/REPO
  github-runner-cli monitor --repo OWNER/REPO
  github-runner-cli refresh --repo OWNER/REPO
  github-runner-cli list-installed [--repo OWNER/REPO]
  github-runner-cli uninstall --repo OWNER/REPO [--dry-run] [--delete-data]
  github-runner-cli uninstall --all [--dry-run] --yes [--delete-data]
  github-runner-cli self-uninstall --yes
  github-runner-cli runner docker-doctor
  github-runner-cli runner <download|register|setup|install-service|start|stop|status|monitor|replace-container> --repo OWNER/REPO [options]
  github-runner-cli host <install|doctor|start|stop|status|monitor|migrate|repair|cleanup|actions-cancel|actions-delete|actions-clear> [--repo OWNER/REPO] [args]

Flags:
  --repo, --repository OWNER/REPO  Repository selection (aliases, including = form)
  --all                           Select all LOCAL managed runner installations
  --yes                           Required for bulk operations and self-uninstall
  --delete-data                   Delete persistent runner registration/workspace volume

Notes:
  list-installed without --repo lists local Docker runner containers and volumes.
  With --repo it also queries GitHub for that repository's runner registrations.
  uninstall refuses busy runners and never deletes images or other Docker volumes.
  self-uninstall removes only the installed CLI and its env/github-runner-cli assets.

Environment:
  RUNNER_BACKEND=docker|macos|auto
  RUNNER_PROFILE=auto|generic|bookbuilder
  RUNNER_DOCKER_SOCKET=0|1  (privileged; explicit opt-in only)
  GITHUB_RUNNER_CLI_ENV=<runtime assets directory>
`)
}

func parseRegisterFlags(args []string) (bool, string, error) {
    allowPublic, name := false, ""
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--allow-public": allowPublic = true
        case "--name":
            if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") { return false, "", errors.New("--name requires a value") }
            i++; name = args[i]
        default: return false, "", fmt.Errorf("unknown option: %s", args[i])
        }
    }
    return allowPublic, name, nil
}

func run(args []string, executable, cwd string) error {
    if len(args) == 0 { usage(); return nil }
    action := strings.ToLower(args[0])
    switch action {
    case "version":
        if err := noExtras(action, args[1:]); err != nil { return err }
        fmt.Println("github-runner-cli "+version); return nil
    case "help", "--help", "-h":
        if err := noExtras(action, args[1:]); err != nil { return err }
        usage(); return nil
    case "self-uninstall":
        return selfUninstall(executable, args[1:])
    }
    env, err := assetsDir(executable, cwd)
    if err != nil { return err }

    switch action {
    case "list-installed":
        repo, rest, err := repoOption(args[1:])
        if err != nil { return err }
        if len(rest) == 1 && rest[0] == "--all" && repo == "" { rest = nil }
        if err := noExtras(action, rest); err != nil { return err }
        return listInstalled(repo)

    case "uninstall":
        repo, rest, err := repoOption(args[1:])
        if err != nil { return err }
        return uninstallCommand(repo, rest)

    case "install":
        repo, rest, err := requireRepo(args[1:])
        if err != nil { return err }
        allow, name, err := parseRegisterFlags(rest)
        if err != nil { return err }
        runner, err := newDockerRunner(repo, env)
        if err != nil { return err }
        if selectedBackend() == "macos" {
            return errors.New("native macOS runner installation is not migrated; use retained host tools")
        }
        return runner.install(allow, name)

    case "start", "stop", "status", "monitor", "refresh":
        repo, rest, err := requireRepo(args[1:])
        if err != nil { return err }
        if err := noExtras(action, rest); err != nil { return err }
        runner, err := newDockerRunner(repo, env)
        if err != nil { return err }
        if selectedBackend() == "macos" {
            oldAction := action
            if action == "refresh" { oldAction = "replace-container" }
            return execute(filepath.Join(env, "runner", "04-run.sh"), []string{repo, oldAction}, "")
        }
        return runner.action(action)

    case "runner":
        if len(args) < 2 { return errors.New("runner action required") }
        sub := strings.ToLower(args[1])
        if sub == "docker-doctor" {
            if err := noExtras(sub, args[2:]); err != nil { return err }
            return dockerDoctor()
        }
        if sub == "setup" || sub == "pr" { return setupWorkflow(args[2:]) }
        repo, rest, err := requireRepo(args[2:])
        if err != nil { return err }
        runner, err := newDockerRunner(repo, env)
        if err != nil { return err }
        if selectedBackend() == "macos" {
            file := "04-run.sh"
            switch sub {
            case "download": file = "01-download.sh"
            case "register", "configure": file = "02-configure.sh"
            case "install-service": rest = append([]string{"install"}, rest...)
            case "uninstall-service": rest = append([]string{"uninstall"}, rest...)
            }
            return execute(filepath.Join(env, "runner", file), append([]string{repo}, rest...), "")
        }
        switch sub {
        case "download":
            if err := noExtras(sub, rest); err != nil { return err }
            return runner.download()
        case "register", "configure":
            allow, name, err := parseRegisterFlags(rest)
            if err != nil { return err }
            return runner.register(allow, name, false)
        case "install-service", "start", "stop", "status", "monitor", "foreground", "replace-container", "uninstall-service":
            if err := noExtras(sub, rest); err != nil { return err }
            return runner.action(sub)
        default: return fmt.Errorf("unknown runner action %q", sub)
        }

    case "host":
        if len(args) < 2 { return errors.New("host action required") }
        sub := strings.ToLower(args[1])
        script, ok := hostActions[sub]
        if !ok { return fmt.Errorf("unknown host action %q", sub) }
        repo, rest, err := repoOption(args[2:])
        if err != nil { return err }
        if strings.HasPrefix(sub, "actions-") { rest = append([]string{strings.TrimPrefix(sub,"actions-")},rest...) }
        if script == "control.sh" { rest = append([]string{sub},rest...) }
        if sub == "cleanup" && len(rest) == 0 { rest = []string{"30"} }
        return execute(filepath.Join(env, script), rest, repo)
    default:
        return fmt.Errorf("unknown command %q (try --help)", action)
    }
}

func main() {
    cwd, e := os.Getwd()
    if e != nil { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
    exe, e := os.Executable()
    if e != nil { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
    if e := run(os.Args[1:], exe, cwd); e != nil {
        fmt.Fprintln(os.Stderr, "ERROR:", e)
        var exit *exec.ExitError
        if errors.As(e, &exit) { os.Exit(exit.ExitCode()) }
        os.Exit(1)
    }
}
