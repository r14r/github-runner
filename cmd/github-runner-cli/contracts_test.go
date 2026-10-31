package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemainingHostAndContainerScriptsParse(t *testing.T) {
    root := filepath.Join("..", "..")
    var paths []string
    for _, pattern := range []string{"scripts/*.sh", "runner/*.sh", "runner/lib/*.sh", "runner/docker/*.sh"} {
        matches, err := filepath.Glob(filepath.Join(root, pattern))
        if err != nil { t.Fatal(err) }
        paths = append(paths, matches...)
    }
    // Check specific runtime contracts; do not depend on a brittle total count.
    for _, expected := range []string{
        "scripts/install.sh", "scripts/control.sh", "scripts/doctor-linux.sh",
        "runner/01-download.sh", "runner/02-configure.sh",
        "runner/04-run.sh", "runner/docker/entrypoint.sh",
    } {
        path := filepath.Join(root, filepath.FromSlash(expected))
        if _, err := os.Stat(path); err != nil {
            t.Errorf("required runtime script missing: %s: %v", expected, err)
        }
    }
    if len(paths) == 0 { t.Fatal("no shell runtime scripts discovered") }
    for _, path := range paths {
        out, err := exec.Command("bash", "-n", path).CombinedOutput()
        if err != nil { t.Errorf("bash syntax %s: %v: %s", path, err, out) }
    }
}

func TestDockerEntrypointRetainsVolumeAndTokenOnStdin(t *testing.T) {
	body, e := os.ReadFile(filepath.Join("..", "..", "runner", "docker", "entrypoint.sh"))
	if e != nil {
		t.Fatal(e)
	}
	s := string(body)
	for _, want := range []string{"cp -a /opt/actions-runner/. /runner/", "read -r token", "--unattended", "gosu runner"} {
		if !strings.Contains(s, want) {
			t.Errorf("Docker entrypoint missing %s", want)
		}
	}
}
func TestBookBuilderImageProfileIncludesAllDoctorRequirements(t *testing.T) {
	body, e := os.ReadFile(filepath.Join("..", "..", "runner", "docker", "Dockerfile"))
	if e != nil {
		t.Fatal(e)
	}
	s := string(body)
	for _, want := range []string{"FROM runner-base AS bookbuilder", "php8.2-cli", "php8.2-curl", "php8.2-mbstring", "php8.2-zip",
		"openjdk-21-jre-headless", "ffmpeg", "fontconfig", "TYPST_VERSION=0.15.1", "JUST_VERSION=1.58.0", "sha256sum -c -"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing BookBuilder image requirement %s", want)
		}
	}
	if strings.Contains(s, "trusted=yes") || strings.Contains(s, "AllowInsecureRepositories") {
		t.Fatal("insecure apt configuration")
	}
}
func TestOnlyGoSourceForWorkflowPatcher(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, file := range []string{"runner/lib/patch_workflow.py", "runner/03-setup.sh", "tests/test_runner.py"} {
		if _, e := os.Stat(filepath.Join(root, file)); !os.IsNotExist(e) {
			t.Errorf("legacy Python workflow implementation still present: %s", file)
		}
	}
}
