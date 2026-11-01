package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRepoFlagsAndAliases(t *testing.T) {
	cases := []struct {
		args  []string
		repo  string
		extra []string
	}{
		{[]string{"--repo", "example-org/demo-app"}, "example-org/demo-app", []string{}},
		{[]string{"--repository", "example-org/demo-app", "--allow-public"}, "example-org/demo-app", []string{"--allow-public"}},
		{[]string{"--repo=example-org/demo-app", "--name", "one"}, "example-org/demo-app", []string{"--name", "one"}},
		{[]string{"--name", "one", "--repository=https://github.com/example-org/demo-app.git"}, "example-org/demo-app", []string{"--name", "one"}},
	}
	for _, tt := range cases {
		repo, extra, err := requireRepo(tt.args)
		if err != nil || repo != tt.repo || !reflect.DeepEqual(extra, tt.extra) {
			t.Errorf("%v: repo=%q extra=%v error=%v", tt.args, repo, extra, err)
		}
	}
}
func TestMissingInvalidDuplicateRepoFlags(t *testing.T) {
	cases := [][]string{
		nil, {"example-org/demo-app"}, {"--repo"}, {"--repo="}, {"--repo", "wrong"},
		{"--repo", "example-org/demo-app", "--repository", "sample-org/second-app"},
		{"--repository", "-bad"}, {"--repo", "owner/repo/extra"},
	}
	for _, args := range cases {
		if _, _, err := requireRepo(args); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
}
func TestUninstallSafeguards(t *testing.T) {
	invalid := []struct {
		repo string
		args []string
	}{
		{"", nil}, {"", []string{"--all"}}, {"example-org/demo-app", []string{"--all", "--yes"}},
		{"example-org/demo-app", []string{"--other"}}, {"", []string{"--delete-data"}},
	}
	for _, tt := range invalid {
		if _, err := parseUninstall(tt.repo, tt.args); err == nil {
			t.Errorf("accepted repo %q flags %v", tt.repo, tt.args)
		}
	}
	valid := []struct {
		repo        string
		args        []string
		all, delete bool
	}{
		{"example-org/demo-app", nil, false, false},
		{"example-org/demo-app", []string{"--delete-data"}, false, true},
		{"", []string{"--all", "--yes"}, true, false},
		{"", []string{"--all", "--yes", "--delete-data"}, true, true},
		{"", []string{"--all", "--dry-run"}, true, false},
	}
	for _, tt := range valid {
		o, err := parseUninstall(tt.repo, tt.args)
		if err != nil || o.All != tt.all || o.DeleteData != tt.delete {
			t.Errorf("%+v: %v", o, err)
		}
	}
}
func TestMatchingRunnerIdentityRequiresExactID(t *testing.T) {
	rec := []runnerRecord{{ID: 42, Name: "imac-example-org-demo-app", Status: "online", Busy: false}}
	r, err := matchedRegistration(runnerIdentity{AgentID: 42, AgentName: rec[0].Name}, rec)
	if err != nil || r == nil || r.ID != 42 {
		t.Fatalf("match: %+v %v", r, err)
	}
	_, err = matchedRegistration(runnerIdentity{AgentID: 43, AgentName: rec[0].Name}, rec)
	if err == nil {
		t.Fatal("mismatched GitHub ID accepted")
	}
	_, err = matchedRegistration(runnerIdentity{AgentName: rec[0].Name}, rec)
	if err == nil {
		t.Fatal("missing runner ID accepted")
	}
	r, err = matchedRegistration(runnerIdentity{AgentID: 42, AgentName: "different"}, rec)
	if err != nil || r != nil {
		t.Fatalf("unexpected match: %+v %v", r, err)
	}
}
func TestSelfUninstallOnlySiblingAssets(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", "")
	dir := t.TempDir()
	binary := filepath.Join(dir, "github-runner-cli")
	assets := filepath.Join(dir, "env", "github-runner-cli")
	if err := os.MkdirAll(filepath.Join(assets, "runner", "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("fake-cli"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "runner", "docker", "Dockerfile"), []byte("FROM ubuntu:24.04"), 0644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other-runner-state")
	if err := os.WriteFile(other, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := selfUninstall(binary, nil); err == nil {
		t.Fatal("removed without --yes")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatal("binary removed on refused operation")
	}
	if err := selfUninstall(binary, []string{"--yes"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{binary, assets} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s unexpectedly remains: %v", p, err)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated runner state changed: %v", err)
	}
}
func TestSelfUninstallRefusesSymlink(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "real-binary")
	_ = os.WriteFile(target, []byte("file"), 0755)
	link := filepath.Join(dir, "github-runner-cli")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if err := selfUninstall(link, []string{"--yes"}); err == nil {
		t.Fatal("accepted symlinked installation")
	}
}
func TestNewCLIMustRejectPositionalRepo(t *testing.T) {
	root := testAssets(t)
	for _, args := range [][]string{
		{"install", "example-org/demo-app"}, {"start", "example-org/demo-app"},
		{"status", "example-org/demo-app"}, {"monitor", "example-org/demo-app"},
		{"runner", "download", "example-org/demo-app"},
		{"runner", "setup", "example-org/demo-app", "--job", "ci"},
		{"uninstall", "example-org/demo-app"},
		{"list-installed", "example-org/demo-app"},
	} {
		if err := run(args, filepath.Join(root, "github-runner-cli"), root); err == nil {
			t.Errorf("accepted positional repository %v", args)
		}
	}
}
func TestSetupRepoFlag(t *testing.T) {
	o, err := parseSetup([]string{"--repository", "example-org/demo-app", "--workflow", ".github/workflows/ci.yml", "--job", "python", "--allow-public"})
	if err != nil || o.repo != "example-org/demo-app" || len(o.workflows) != 1 || o.jobs[0] != "python" || !o.allowPublic {
		t.Fatalf("parseSetup: %+v %v", o, err)
	}
	if _, err := parseSetup([]string{"example-org/demo-app", "--job", "ci"}); err == nil {
		t.Fatal("positional setup repository accepted")
	}
}
func TestDockerNamingAndExactScope(t *testing.T) {
	if !strings.HasPrefix(runnerContainerPrefix, "github-runner-") {
		t.Fatal("wrong container prefix")
	}
	r, err := newDockerRunner("example-org/demo-app", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if r.Container != "github-runner-example-org-demo-app" || r.Volume != "github-runner-example-org-demo-app-state" {
		t.Fatalf("existing Docker names changed: %+v", r)
	}
}
