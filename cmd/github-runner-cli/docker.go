package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const dockerPlatform = "linux/amd64"

type dockerRunner struct {
	Repo      string
	Slug      string
	Profile   string
	Image     string
	Volume    string
	Container string
	Assets    string
}

func newDockerRunner(repo, assets string) (dockerRunner, error) {
	slug, err := repoSlug(repo)
	if err != nil {
		return dockerRunner{}, err
	}
	profile := os.Getenv("RUNNER_PROFILE")
	switch profile {
	case "", "auto":
		profile = "runner-base"
		if slug == "r14r-bookbuilder" {
			profile = "bookbuilder"
		}
	case "generic":
		profile = "runner-base"
	case "runner-base", "bookbuilder":
	default:
		return dockerRunner{}, fmt.Errorf("invalid RUNNER_PROFILE: %s", profile)
	}
	image := os.Getenv("RUNNER_IMAGE")
	if image == "" {
		image = "github-actions-runner-local:2.338.1"
		if profile == "bookbuilder" {
			image += "-bookbuilder"
		}
	}
	return dockerRunner{
		Repo: gitHubRepository(repo), Slug: slug, Profile: profile, Image: image,
		Volume: "github-runner-" + slug + "-state", Container: "github-runner-" + slug, Assets: assets,
	}, nil
}
func (r dockerRunner) docker(args ...string) (string, error) {
	out, err := commandOutput("", "docker", args...)
	return out, err
}
func (r dockerRunner) exists(kind, name string) bool {
	return exec.Command("docker", kind, "inspect", name).Run() == nil
}
func (r dockerRunner) stateConfigured() bool {
	if !r.exists("volume", r.Volume) {
		return false
	}
	err := exec.Command("docker", "run", "--rm", "--platform", dockerPlatform,
		"--mount", "type=volume,source="+r.Volume+",target=/runner", r.Image, "check").Run()
	return err == nil
}
func (r dockerRunner) selectPlatform() error {
	if selectedBackend() != "docker" {
		return errors.New("native macOS backend still uses the explicitly retained legacy host tools")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		return errors.New("Docker daemon unavailable (start Colima and select docker context colima)")
	}
	arch, err := r.docker("info", "--format", "{{.Architecture}}")
	if err != nil {
		return err
	}
	if arch != "x86_64" && arch != "amd64" {
		return fmt.Errorf("Docker host %s is not linux/amd64", arch)
	}
	return nil
}
func dockerDoctor() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return err
	}
	arch, err := commandOutput("", "docker", "info", "--format", "{{.Architecture}}")
	if err != nil {
		return err
	}
	if arch != "x86_64" && arch != "amd64" {
		return fmt.Errorf("unsupported Docker architecture: %s", arch)
	}
	context, err := commandOutput("", "docker", "context", "show")
	if err != nil {
		return err
	}
	fmt.Println("Docker context:", context)
	if err := commandRun("", "docker", "system", "df"); err != nil {
		return err
	}
	if context == "colima" {
		if _, err := exec.LookPath("colima"); err == nil {
			fmt.Println("Colima VM filesystems:")
			_ = commandRun("", "colima", "ssh", "--", "df", "-h")
		}
	}
	free, err := commandOutput("", "docker", "run", "--rm", "--platform", dockerPlatform,
		"--entrypoint", "sh", "ubuntu:24.04", "-c", "df -Pk / | awk 'NR==2 {print $4}'")
	if err != nil {
		return err
	}
	kb, err := strconv.ParseInt(strings.TrimSpace(free), 10, 64)
	if err != nil {
		return fmt.Errorf("cannot parse container free space %q: %w", free, err)
	}
	if kb < 2*1024*1024 {
		return fmt.Errorf("Docker has only %d MiB free; at least 2 GiB required", kb/1024)
	}
	fmt.Printf("Container writable storage free: %d MiB\n", kb/1024)
	fmt.Println("Checking signed Ubuntu package indexes ...")
	if err := commandRun("", "docker", "run", "--rm", "--platform", dockerPlatform,
		"ubuntu:24.04", "sh", "-ec", "apt-get update"); err != nil {
		return fmt.Errorf("Ubuntu APT signature verification failed. Check Colima disk usage and network; do not disable signature checks: %w", err)
	}
	return nil
}
func (r dockerRunner) download() error {
	if err := r.selectPlatform(); err != nil {
		return err
	}
	if err := dockerDoctor(); err != nil {
		return err
	}
	dockerfile := filepath.Join(r.Assets, "runner", "docker", "Dockerfile")
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("Dockerfile not installed: %w", err)
	}
	fmt.Printf("Building %s (profile %s)\n", r.Image, r.Profile)
	if err := commandRun("", "docker", "build", "--platform", dockerPlatform, "--target", r.Profile,
		"--tag", r.Image, "--file", dockerfile, filepath.Join(r.Assets, "runner", "docker")); err != nil {
		return err
	}
	if r.Profile == "bookbuilder" {
		doctorPath := filepath.Join(r.Assets, "doctor-linux.sh")
		body, err := os.ReadFile(doctorPath)
		if err != nil {
			return err
		}
		cmd := exec.Command("docker", "run", "--rm", "--interactive", "--platform", dockerPlatform, "--entrypoint", "bash", r.Image, "-s")
		cmd.Stdin = bytes.NewReader(body)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("BookBuilder doctor failed: %w", err)
		}
	}
	if err := commandRun("", "docker", "volume", "create", r.Volume); err != nil {
		return err
	}
	fmt.Printf("Image ready: %s (registration volume %s)\n", r.Image, r.Volume)
	return nil
}
func (r dockerRunner) register(allowPublic bool, name string, ifNeeded bool) error {
	if err := r.selectPlatform(); err != nil {
		return err
	}
	if !r.exists("image", r.Image) {
		return fmt.Errorf("image %s missing: download first", r.Image)
	}
	if !r.exists("volume", r.Volume) {
		return fmt.Errorf("volume %s missing: download first", r.Volume)
	}
	if err := commandRun("", "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh auth login required: %w", err)
	}
	visibility, err := commandOutput("", "gh", "repo", "view", r.Repo, "--json", "visibility", "--jq", ".visibility")
	if err != nil {
		return err
	}
	if strings.EqualFold(visibility, "PUBLIC") && !allowPublic {
		return errors.New("public repository: use --allow-public only on an isolated trusted runner")
	}
	if r.stateConfigured() {
		if ifNeeded {
			fmt.Printf("Existing registration preserved in %s\n", r.Volume)
			return nil
		}
		return fmt.Errorf("already registered in %s", r.Volume)
	}
	if name == "" {
		host, err := os.Hostname()
		if err != nil {
			return err
		}
		host = strings.ToLower(regexp.MustCompile(`[^a-z0-9_.-]`).ReplaceAllString(host, ""))
		name = host + "-" + r.Slug
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(name) {
		return errors.New("invalid runner name")
	}
	token, err := commandOutput("", "gh", "api", "--method", "POST", "repos/"+r.Repo+"/actions/runners/registration-token", "--jq", ".token")
	if err != nil {
		return fmt.Errorf("request runner registration token: %w", err)
	}
	if token == "" || token == "null" {
		return errors.New("GitHub returned empty token")
	}
	cmd := exec.Command("docker", "run", "--rm", "--interactive", "--platform", dockerPlatform,
		"--mount", "type=volume,source="+r.Volume+",target=/runner",
		"--env", "RUNNER_REPOSITORY="+r.Repo, "--env", "RUNNER_NAME="+name,
		"--env", "RUNNER_LABEL=repo-"+r.Slug, r.Image, "configure")
	// Never print token or put it in argv/environment, only pass on stdin.
	cmd.Stdin = strings.NewReader(token + "\n")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("Registered %s with [self-hosted, Linux, X64, repo-%s]\n", name, r.Slug)
	return nil
}
func (r dockerRunner) imageMatches() bool {
	actual, e := r.docker("container", "inspect", "--format", "{{.Image}}", r.Container)
	if e != nil {
		return false
	}
	desired, e := r.docker("image", "inspect", "--format", "{{.Id}}", r.Image)
	return e == nil && actual == desired
}
func (r dockerRunner) createContainer() error {
	socket := os.Getenv("RUNNER_DOCKER_SOCKET")
	if socket != "" && socket != "0" && socket != "1" {
		return errors.New("RUNNER_DOCKER_SOCKET must be 0 or 1")
	}
	args := []string{"create", "--platform", dockerPlatform, "--name", r.Container,
		"--restart", "unless-stopped", "--mount", "type=volume,source=" + r.Volume + ",target=/runner"}
	if socket == "1" {
		fmt.Fprintln(os.Stderr, "WARNING: runner will have full Docker daemon access")
		args = append(args, "--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock")
	}
	args = append(args, r.Image, "run")
	return commandRun("", "docker", args...)
}
func (r dockerRunner) ensureRegistered() error {
	if !r.stateConfigured() {
		return fmt.Errorf("runner not registered in %s: install first", r.Volume)
	}
	return nil
}
func (r dockerRunner) installContainer() error {
	if err := r.ensureRegistered(); err != nil {
		return err
	}
	if r.exists("container", r.Container) {
		fmt.Println("Container already exists:", r.Container)
		if !r.imageMatches() {
			fmt.Println("WARNING: image changed: stop idle runner and run refresh")
		}
		return nil
	}
	return r.createContainer()
}
func (r dockerRunner) refresh() error {
	if err := r.ensureRegistered(); err != nil {
		return err
	}
	if !r.exists("image", r.Image) {
		return fmt.Errorf("image %s missing", r.Image)
	}
	if r.exists("container", r.Container) {
		running, err := r.docker("container", "inspect", "--format", "{{.State.Running}}", r.Container)
		if err != nil {
			return err
		}
		if running == "true" {
			return errors.New("runner still running; check GitHub busy status and stop it before refresh")
		}
		mounts, err := r.docker("container", "inspect", "--format", `{{range .Mounts}}{{if eq .Destination "/var/run/docker.sock"}}yes{{end}}{{end}}`, r.Container)
		if err != nil {
			return err
		}
		if mounts == "yes" && os.Getenv("RUNNER_DOCKER_SOCKET") != "1" {
			return errors.New("runner has Docker socket mount; set RUNNER_DOCKER_SOCKET=1 explicitly to preserve its privileges")
		}
		if err := commandRun("", "docker", "rm", r.Container); err != nil {
			return err
		}
	}
	if err := r.createContainer(); err != nil {
		return err
	}
	fmt.Printf("Container refreshed; persistent volume %s preserved\n", r.Volume)
	return nil
}
func (r dockerRunner) action(action string) error {
	if err := r.selectPlatform(); err != nil {
		return err
	}
	if err := r.ensureRegistered(); err != nil {
		return err
	}
	switch action {
	case "install-service":
		return r.installContainer()
	case "refresh", "replace-container":
		return r.refresh()
	case "start":
		if !r.exists("container", r.Container) {
			if err := r.createContainer(); err != nil {
				return err
			}
		}
		if !r.imageMatches() {
			return errors.New("outdated container image: stop and refresh before starting")
		}
		return commandRun("", "docker", "start", r.Container)
	case "stop":
		return commandRun("", "docker", "stop", r.Container)
	case "uninstall-service":
		if !r.exists("container", r.Container) {
			return nil
		}
		if err := commandRun("", "docker", "rm", "-f", r.Container); err != nil {
			return err
		}
		fmt.Println("Registration volume retained:", r.Volume)
		return nil
	case "status":
		if r.exists("container", r.Container) {
			if err := commandRun("", "docker", "container", "inspect", "--format",
				"{{.Name}}: {{.State.Status}} (restart: {{.HostConfig.RestartPolicy.Name}})", r.Container); err != nil {
				return err
			}
			if !r.imageMatches() {
				fmt.Println("WARNING: container image outdated")
			}
		} else {
			fmt.Println("Docker container not installed:", r.Container)
		}
		return r.githubStatus()
	case "monitor":
		for {
			fmt.Println("\n--- Runner status ---")
			if err := r.action("status"); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			fmt.Println("--- Recent GitHub Actions runs ---")
			_ = commandRun("", "gh", "run", "list", "--repo", r.Repo, "--limit", "5")
			_ = commandRun("", "docker", "logs", "--tail", "8", r.Container)
			interval := 10
			if value := os.Getenv("RUNNER_MONITOR_SECONDS"); value != "" {
				n, e := strconv.Atoi(value)
				if e != nil || n < 2 {
					return errors.New("RUNNER_MONITOR_SECONDS must be integer >= 2")
				}
				interval = n
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
	case "foreground":
		if !r.exists("container", r.Container) {
			if err := r.createContainer(); err != nil {
				return err
			}
		}
		return commandRun("", "docker", "start", "--attach", r.Container)
	default:
		return fmt.Errorf("unsupported runner action: %s", action)
	}
}
func (r dockerRunner) githubStatus() error {
	cmd := exec.Command("docker", "run", "--rm", "--platform", dockerPlatform,
		"--mount", "type=volume,source="+r.Volume+",target=/runner", r.Image, "identity")
	identity, e := cmd.Output()
	if e != nil {
		fmt.Println("Unable to read runner identity")
		return nil
	}
	name := strings.TrimSpace(string(identity))
	if name == "" {
		return nil
	}
	data, e := commandOutput("", "gh", "api", "--paginate", "repos/"+r.Repo+"/actions/runners?per_page=100")
	if e != nil {
		fmt.Println("GitHub runner API status unavailable")
		return nil
	}
	var parsed struct {
		Runners []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Busy   bool   `json:"busy"`
		} `json:"runners"`
	}
	if json.Unmarshal([]byte(data), &parsed) == nil {
		for _, x := range parsed.Runners {
			if x.Name == name {
				fmt.Printf("GitHub status: %s; busy=%t; name=%s\n", x.Status, x.Busy, name)
				return nil
			}
		}
	}
	fmt.Printf("GitHub runner %s not found\n", name)
	return nil
}
func (r dockerRunner) install(allowPublic bool, name string) error {
	if err := r.download(); err != nil {
		return err
	}
	if err := r.register(allowPublic, name, true); err != nil {
		return err
	}
	return r.installContainer()
}

// The official runner container entrypoint remains inside the Docker image.
