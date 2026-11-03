package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testAssets(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"runner/docker/Dockerfile", "runner/docker/entrypoint.sh"} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if e := os.MkdirAll(filepath.Dir(full), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(full, []byte("fixture"), 0644); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("GITHUB_RUNNER_CLI_ENV", root)
	return root
}
func TestHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"version"}} {
		if e := run(args, "/tmp/github-runner-cli", t.TempDir()); e != nil {
			t.Fatal(e)
		}
	}
}
func TestAssetsError(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", filepath.Join(t.TempDir(), "missing"))
	if _, e := assetsDir("/tmp/cli", t.TempDir()); e == nil {
		t.Fatal("missing assets accepted")
	}
}
func TestBadArgumentsDoNotModifyRunnerState(t *testing.T) {
	root := testAssets(t)
	for _, args := range [][]string{
		{"install"}, {"install", "example-org/demo-app", "--bad"},
		{"install", "example-org/demo-app", "--name"},
		{"start"}, {"stop", "example-org/demo-app", "extra"},
		{"runner"}, {"runner", "unknown", "example-org/demo-app"},
		{"runner", "register", "invalid"},
	} {
		if e := run(args, filepath.Join(root, "cli"), root); e == nil {
			t.Errorf("accepted invalid %v", args)
		}
	}
}
func TestRepositoryProfiles(t *testing.T) {
	t.Setenv("RUNNER_PROFILE", "")
	t.Setenv("RUNNER_IMAGE", "")
	a, e := newDockerRunner("example-org/demo-app", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if a.Profile != "toolchain" || a.Image != "github-actions-runner-local:2.338.1-example-org-demo-app" {
		t.Fatalf("%+v", a)
	}
	b, e := newDockerRunner("sample-org/second-app", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if b.Profile != "toolchain" || b.Container != "github-runner-sample-org-second-app" {
		t.Fatalf("%+v", b)
	}
	if a.Volume == b.Volume {
		t.Fatal("different repositories must not share registration volumes")
	}
}
func TestExplicitProfileAndImage(t *testing.T) {
	t.Setenv("RUNNER_PROFILE", "toolchain")
	t.Setenv("RUNNER_IMAGE", "test:image")
	r, e := newDockerRunner("other/project", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if r.Profile != "toolchain" || r.Image != "test:image" {
		t.Fatal(r)
	}
}
func TestProfileValidation(t *testing.T) {
	t.Setenv("RUNNER_PROFILE", "unknown")
	if _, e := newDockerRunner("example-org/demo-app", t.TempDir()); e == nil {
		t.Fatal("invalid profile accepted")
	}
}
func TestRepoSlug(t *testing.T) {
	slug, e := repoSlug("https://github.com/example-org/demo-app.git")
	if e != nil || slug != "example-org-demo-app" {
		t.Fatalf("%q %v", slug, e)
	}
	for _, name := range []string{"", "bad", "../../etc/passwd", "x/y/z"} {
		if _, e := repoSlug(name); e == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
func TestRegisterFlagParsing(t *testing.T) {
	allow, name, e := parseRegisterFlags([]string{"--allow-public", "--name", "runner-1"})
	if e != nil || !allow || name != "runner-1" {
		t.Fatalf("%v %q %v", allow, name, e)
	}
	for _, args := range [][]string{{"--name"}, {"--name", "--allow-public"}, {"--bogus"}} {
		if _, _, e := parseRegisterFlags(args); e == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func TestSetupOptions(t *testing.T) {
	o, e := parseSetup([]string{"--repo", "example-org/demo-app", "--workflow", ".github/workflows/ci.yml", "--job", "python", "--allow-public"})
	if e != nil || o.repo != "example-org/demo-app" || len(o.workflows) != 1 || o.jobs[0] != "python" || !o.allowPublic {
		t.Fatalf("%+v: %v", o, e)
	}
	for _, args := range [][]string{nil, {"bad"}, {"--repo", "example-org/demo-app", "--workflow"}, {"--repo", "example-org/demo-app", "--unknown"}} {
		if _, e := parseSetup(args); e == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func TestGoOnlyJustfile(t *testing.T) {
	raw, e := os.ReadFile(filepath.Join("..", "..", "justfile"))
	if e != nil {
		t.Fatal(e)
	}
	just := string(raw)
	for _, recipe := range []string{"\nbootstrap:", "\nci:", "\nbuild:", "\ninstall:"} {
		if !strings.Contains(just, recipe) {
			t.Errorf("recipe %q missing", recipe)
		}
	}
	for _, forbidden := range []string{"pytest", "python3", "patch_workflow.py"} {
		if strings.Contains(just, forbidden) {
			t.Errorf("Python dependency remains: %s", forbidden)
		}
	}
}
