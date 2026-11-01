package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	defer func() { os.Stdout = old; _ = read.Close() }()
	// Deliberately no runtime assets: detailed help must be stand-alone.
	err = run(args, filepath.Join(t.TempDir(), "no-installed-binary"), t.TempDir())
	_ = write.Close()
	data, readErr := io.ReadAll(read)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), err
}

func TestEveryCommandHasHelpfulDetailedDocumentation(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", filepath.Join(t.TempDir(), "missing-assets"))
	for name, doc := range commandDocs {
		if doc.Summary == "" || doc.Description == "" || len(doc.Usage) == 0 || len(doc.Examples) == 0 || len(doc.Options) == 0 {
			t.Errorf("%q missing summary/description/usage/examples/options", name)
		}
		path := strings.Split(name, " ")
		text, err := captureCLI(t, append([]string{"help"}, path...)...)
		if err != nil {
			t.Errorf("help for %q failed: %v", name, err)
			continue
		}
		for _, expected := range []string{"USAGE:", "OPTIONS:", "EXAMPLES:", doc.Summary} {
			if !strings.Contains(text, expected) {
				t.Errorf("help for %q missing %q", name, expected)
			}
		}
	}
}

func TestHelpWorksBeforeAssetsAndRequiredRepo(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", filepath.Join(t.TempDir(), "does-not-exist"))
	cases := []struct {
		args     []string
		expected []string
	}{
		{[]string{"help"}, []string{"RUNNER COMMANDS:", "MORE HELP:", "GLOBAL ENVIRONMENT:"}},
		{[]string{"--help"}, []string{"RUNNER COMMANDS:", "ghr help status"}},
		{[]string{"-h"}, []string{"RUNNER COMMANDS:"}},
		{[]string{"help", "status"}, []string{"ghr status --repo OWNER/REPO", "GitHub status: online or offline", "EXAMPLES:"}},
		{[]string{"status", "--help"}, []string{"ghr status --repo OWNER/REPO", "Help works without --repo"}},
		{[]string{"status", "-h"}, []string{"ghr status --repo OWNER/REPO"}},
		{[]string{"status", "--repo", "example-org/demo-app", "--help"}, []string{"ghr status --repo OWNER/REPO"}},
		{[]string{"uninstall", "--help"}, []string{"--dry-run", "--delete-data", "--all", "--yes"}},
		{[]string{"self-uninstall", "--help"}, []string{"--yes", "Does NOT stop/unregister"}},
		{[]string{"runner", "--help"}, []string{"Subcommands:", "docker-doctor"}},
		{[]string{"runner", "help"}, []string{"Subcommands:", "docker-doctor"}},
		{[]string{"runner", "help", "setup"}, []string{"--workflow PATH", "--job ID", "--allow-cross-os"}},
		{[]string{"help", "runner", "setup"}, []string{"--workflow PATH", "--job ID", "--allow-cross-os"}},
		{[]string{"runner", "setup", "--help"}, []string{"ghr runner setup --repo OWNER/REPO", "--no-merge"}},
		{[]string{"runner", "download", "--repo", "example-org/demo-app", "--help"}, []string{"ghr runner download --repo OWNER/REPO"}},
		{[]string{"runner", "docker-doctor", "--help"}, []string{"2 GiB", "amd64"}},
		{[]string{"version", "--help"}, []string{"ghr version", "OPTIONS:"}},
		{[]string{"list-installed", "--help"}, []string{"--all", "--repository", "current Docker"}},
	}
	for _, tt := range cases {
		text, err := captureCLI(t, tt.args...)
		if err != nil {
			t.Errorf("%v returned %v", tt.args, err)
			continue
		}
		for _, expected := range tt.expected {
			if !strings.Contains(text, expected) {
				t.Errorf("%v missing %q in %q", tt.args, expected, text)
			}
		}
	}
}

func TestHelpRejectsUnknownTopicsWithoutSideEffects(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", filepath.Join(t.TempDir(), "missing"))
	for _, args := range [][]string{
		{"help", "unknown"},
		{"help", "runner", "unknown"},
		{"help", "host", "unknown"},
		{"runner", "no-such-command", "--help"},
		{"host", "no-such-command", "--help"},
		{"help", "status", "wrong"},
	} {
		_, err := captureCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), "unknown help topic") {
			t.Errorf("%v unexpectedly succeeded or returned wrong error: %v", args, err)
		}
	}
}

func TestHelpAliasesDocumentAllImplementedActions(t *testing.T) {
	for _, name := range []string{
		"help", "version", "install", "start", "stop", "status",
		"monitor", "refresh", "list-installed", "uninstall", "self-uninstall",
		"runner", "runner docker-doctor", "runner download", "runner register",
		"runner configure", "runner setup", "runner pr", "runner install-service",
		"runner start", "runner stop", "runner status", "runner monitor",
		"runner foreground", "runner replace-container", "runner uninstall-service",

	} {
		if _, ok := commandDocs[name]; !ok {
			t.Errorf("no documentation for %s", name)
		}
	}
}
