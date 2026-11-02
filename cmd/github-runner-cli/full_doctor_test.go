package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeFullDoctor(failing string, calls *[]string) doctorChecks {
	check := func(name string) error {
		*calls = append(*calls, name)
		if name == failing {
			return errors.New("synthetic " + name + " failure")
		}
		return nil
	}
	return doctorChecks{
		container: func(io.Writer) error { return check("container") },
		toolchain: func(io.Writer) error { return check("toolchain") },
		registration: func(io.Writer) (runnerIdentity, error) {
			if err := check("registration"); err != nil {
				return runnerIdentity{}, err
			}
			return runnerIdentity{AgentID: 42, AgentName: "example-runner"}, nil
		},
		github: func(_ io.Writer, identity runnerIdentity) error {
			if identity.AgentID != 42 {
				return errors.New("wrong identity forwarded")
			}
			return check("github")
		},
		workflows: func(io.Writer) error { return check("workflows") },
		variable:  func(io.Writer) error { return check("variable") },
	}
}

func TestFullDoctorSixChecksPass(t *testing.T) {
	var output bytes.Buffer
	calls := []string{}
	if err := runFullDoctor(&output, "example-org/demo", fakeFullDoctor("", &calls)); err != nil {
		t.Fatal(err)
	}
	expected := []string{"container", "toolchain", "registration", "github", "workflows", "variable"}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("check order %v, expected %v", calls, expected)
	}
	for _, heading := range []string{
		"01", "Docker container", "02", "Image and toolchain", "03", "Local runner registration",
		"04", "GitHub runner status", "05", "Workflow runner selectors", "06", "RUNNER_LABELS_JSON",
		"RESULT: PASS — all 6 checks OK",
	}
	} {
		if !strings.Contains(output.String(), heading) {
			t.Errorf("missing %q in %q", heading, output.String())
		}
	}
}

func TestFullDoctorContinuesIndependentChecksOnFailure(t *testing.T) {
	for _, which := range []string{"container", "toolchain", "workflows", "variable"} {
		t.Run(which, func(t *testing.T) {
			var out bytes.Buffer
			calls := []string{}
			err := runFullDoctor(&out, "example-org/demo", fakeFullDoctor(which, &calls))
			if err == nil || !strings.Contains(err.Error(), which) {
				t.Fatalf("expected %s failure, got %v", which, err)
			}
			if len(calls) != 6 {
				t.Errorf("independent checks skipped after failure: %v", calls)
			}
			if !strings.Contains(out.String(), "RESULT: FAIL — 1 of 6") {
				t.Errorf("bad summary: %s", out.String())
			}
		})
	}
}

func TestFullDoctorDoesNotQueryRemoteForUnknownIdentity(t *testing.T) {
	var out bytes.Buffer
	calls := []string{}
	err := runFullDoctor(&out, "example-org/demo", fakeFullDoctor("registration", &calls))
	if err == nil || !strings.Contains(err.Error(), "registration") {
		t.Fatalf("registration failure: %v", err)
	}
	for _, name := range calls {
		if name == "github" {
			t.Fatal("queried GitHub without verified local identity")
		}
	}
	if !strings.Contains(out.String(), "RESULT: FAIL — 2 of 6") {
		t.Error(out.String())
	}
	if !strings.Contains(out.String(), "local registration not verified") {
		t.Error(out.String())
	}
}

func TestDoctorRunnerOnlineIdleAndLabels(t *testing.T) {
	rec := runnerRecord{ID: 42, Name: "example", Status: "online", Busy: false}
	for _, label := range []string{"self-hosted", "Linux", "X64", "repo-example-org-demo"} {
		rec.Labels = append(rec.Labels, struct {
			Name string `json:"name"`
		}{Name: label})
	}
	if err := verifyGitHubDoctorRunner(rec, "example-org-demo"); err != nil {
		t.Fatal(err)
	}
	rec.Busy = true
	if err := verifyGitHubDoctorRunner(rec, "example-org-demo"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("expected busy runner status: %v", err)
	}
	rec.Busy = false
	rec.Status = "offline"
	if err := verifyGitHubDoctorRunner(rec, "example-org-demo"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("expected offline error: %v", err)
	}
	rec.Status = "online"
	rec.Labels = rec.Labels[:3]
	if err := verifyGitHubDoctorRunner(rec, "example-org-demo"); err == nil || !strings.Contains(err.Error(), "repo-example-org-demo") {
		t.Fatalf("missing label should fail: %v", err)
	}
}

func TestDoctorRunnerVariableValidation(t *testing.T) {
	valid := `["self-hosted","Linux","X64","repo-example-org-demo"]`
	if err := validateDoctorRunnerVariable(valid, "example-org-demo"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		`"ubuntu-24.04"`, `["self-hosted","Linux"]`, `["self-hosted","Linux","X64","repo-other-app"]`,
		`not-json`, `{"self-hosted":true}`, `[]`,
	} {
		if err := validateDoctorRunnerVariable(value, "example-org-demo"); err == nil {
			t.Errorf("accepted wrong runner routing value %s", value)
		}
	}
}

func TestDoctorWorkflowSourceVariableAndReusableJobs(t *testing.T) {
	input := `name: Demo
on: [push]
jobs:
  check:
    runs-on: ${{ fromJSON(vars.RUNNER_LABELS_JSON || '"ubuntu-24.04"') }}
    steps:
      - run: echo hello
  build:
    runs-on: ${{ fromJSON(vars.RUNNER_LABELS_JSON || '"ubuntu-24.04"') }}
    strategy:
      matrix:
        os: [one, two]
  reused:
    uses: example-org/workflows/.github/workflows/shared.yml@main
`
	// Keep the expression source literal: JavaScript template escaping is
	// used only in the connector write call, not in the resulting Go file.
	result := checkDoctorWorkflowSource("demo.yaml", []byte(input))
	if len(result.problems) != 0 || result.checked != 2 || result.reusable != 1 || result.count != 3 {
		t.Fatalf("unexpected workflow check: %+v", result)
	}
}

func TestDoctorWorkflowSourceRejectsStaticMissingAndCommentedSelectors(t *testing.T) {
	input := `jobs:
  fixed:
    runs-on: ubuntu-latest
  missing:
    steps:
      - run: echo ok
  misleading:
    # runs-on: ${{ fromJSON(vars.RUNNER_LABELS_JSON) }}
    runs-on: [self-hosted, Linux, X64]
  dynamic:
    runs-on: ${{ matrix.os }}
`
	result := checkDoctorWorkflowSource("ci.yml", []byte(input))
	if result.checked != 0 || len(result.problems) != 4 {
		t.Fatalf("static or missing selectors mistakenly passed: %+v", result)
	}
}

func TestDoctorWorkflowDirectoryChecksYmlAndYaml(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	valid := "jobs:\n  test:\n    runs-on: ${{ fromJSON(vars.RUNNER_LABELS_JSON) }}\n"
	for _, name := range []string{"ci.yml", "deploy.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(valid), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := checkDoctorWorkflowDirectory(root, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ci.yml", "deploy.yaml"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("workflow not reported %s", name)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("jobs:\n  deploy:\n    runs-on: ubuntu-24.04\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkDoctorWorkflowDirectory(root, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "deploy") {
		t.Fatalf("expected failure for second .yaml file: %v", err)
	}
}

func TestDoctorWorkflowDirectoryRejectsNoWorkflows(t *testing.T) {
	if err := checkDoctorWorkflowDirectory(t.TempDir(), &bytes.Buffer{}); err == nil {
		t.Fatal("missing workflows passed")
	}
}

func TestDoctorWorkflowDirectoryRejectsNonRegularFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".github", "workflows")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "ci.yml")
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), target); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := checkDoctorWorkflowDirectory(filepath.Dir(filepath.Dir(dir)), &bytes.Buffer{}); err == nil {
		t.Fatal("workflow symlink accepted")
	}
}

func Example_validateDoctorRunnerVariable() {
	err := validateDoctorRunnerVariable(`["self-hosted","Linux","X64","repo-example-org-demo"]`, "example-org-demo")
	fmt.Println(err == nil)
	// Output: true
}

func TestDoctorCompactLayoutNoHeadingOrNestedOutput(t *testing.T) {
	var out bytes.Buffer
	calls := []string{}
	checks := fakeFullDoctor("", &calls)
	checks.container = func(w io.Writer) error {
		fmt.Fprintln(w, "    Container: github-runner-example")
		fmt.Fprintln(w, "    State:     running")
		fmt.Fprintln(w, "    Image:     example:latest (matches running container)")
		return nil
	}
	checks.toolchain = func(w io.Writer) error {
		fmt.Fprintln(w, "GitHub Runner — Toolchain Doctor")
		fmt.Fprintln(w, "01: Clone repository")
		fmt.Fprintln(w, "    Workflows:  6")
		fmt.Fprintln(w, "    Actions:    10")
		fmt.Fprintln(w, "    Tools:      4")
		fmt.Fprintln(w, "RESULT: PASS — 0 tools verified in image, 4 provided by workflow actions")
		fmt.Fprintln(w, "Repository: example-org/demo")
		fmt.Fprintln(w, "Image: example:latest")
		return nil
	}
	checks.registration = func(w io.Writer) (runnerIdentity, error) {
		fmt.Fprintln(w, "    Name: example-agent")
		fmt.Fprintln(w, "    ID: 42")
		fmt.Fprintln(w, "    Volume: example-state")
		return runnerIdentity{AgentID: 42, AgentName: "example-agent"}, nil
	}
	checks.github = func(w io.Writer, _ runnerIdentity) error {
		fmt.Fprintln(w, "    Name: example-agent")
		fmt.Fprintln(w, "    Status: online")
		fmt.Fprintln(w, "    Busy: false")
		return nil
	}
	checks.workflows = func(w io.Writer) error {
		fmt.Fprintln(w, "    ci.yml                        1/1 jobs use variable")
		fmt.Fprintln(w, "    pages.yaml                    2/2 jobs use variable")
		return nil
	}
	checks.variable = func(w io.Writer) error {
		fmt.Fprintln(w, `    RUNNER_LABELS_JSON: ["self-hosted","Linux","X64","repo-example-org-demo"]`)
		return nil
	}
	if err := runFullDoctorWithColor(&out, "example-org/demo", checks, false); err != nil { t.Fatal(err) }
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 { t.Fatalf("expected exactly six rows and summary (no headers/details), got %d: %q", len(lines), out.String()) }
	for i := 0; i < 6; i++ {
		expected := fmt.Sprintf("%02d    OK    ", i+1)
		if !strings.HasPrefix(lines[i], expected) {
			t.Errorf("line %d is not three-column: %q (want prefix %q)", i, lines[i], expected)
		}
	}
	if !strings.Contains(lines[1], "0 tools verified in image, 4 provided by workflow actions") || !strings.Contains(lines[1], "6 workflows") {
		t.Errorf("toolchain summary missing: %s", lines[1])
	}
	if !strings.Contains(lines[4], "2 files, 3/3 jobs") {
		t.Errorf("workflow summary missing: %s", lines[4])
	}
	for _, forbidden := range []string{"GitHub Runner — Full Doctor", "GitHub Runner — Toolchain Doctor", "01: Clone repository", "\nRepository:", "\nImage:"} {
		if strings.Contains(out.String(), forbidden) { t.Errorf("nested title or unaligned output %q: %s", forbidden, out.String()) }
	}
}

func TestDoctorStatusBadgesColorAndAlignment(t *testing.T) {
	for _, status := range []bool{true, false} {
		plain := doctorBadge(status, false)
		colored := doctorBadge(status, true)
		if len(plain) != 6 { t.Errorf("status badge not fixed width: %q", plain) }
		if !strings.Contains(colored, "\x1b[") || !strings.HasSuffix(colored, "\x1b[0m") { t.Errorf("missing terminal color/reset: %q", colored) }
		if !strings.Contains(colored, plain) { t.Errorf("colored badge changes visible width: %q", colored) }
	}
	calls := []string{}
	var out bytes.Buffer
	if err := runFullDoctorWithColor(&out, "test/repo", fakeFullDoctor("variable", &calls), true); err == nil { t.Fatal("failed doctor must return an error") }
	if !strings.Contains(out.String(), "\x1b[97;41m FAIL \x1b[0m") { t.Fatalf("missing red error background: %q", out.String()) }
	if !strings.Contains(out.String(), "\x1b[30;42m  OK  \x1b[0m") { t.Fatalf("missing green success background: %q", out.String()) }
}

func TestDoctorErrorIsReportedOnceInRowAndSummary(t *testing.T) {
	var out bytes.Buffer
	calls := []string{}
	err := runFullDoctor(&out, "test/repo", fakeFullDoctor("registration", &calls))
	var diagnosis *doctorFailure
	if !errors.As(err, &diagnosis) { t.Fatalf("expected recognized doctorFailure type, got %v", err) }
	if !strings.Contains(out.String(), "03   FAIL   Local runner registration — synthetic registration failure") {
		t.Errorf("missing compact error detail: %s", out.String())
	}
	if !strings.Contains(out.String(), "04   FAIL   GitHub runner status and labels — local registration not verified") {
		t.Errorf("missing dependent check explanation: %s", out.String())
	}
	if !strings.Contains(out.String(), "RESULT: FAIL — 2 of 6") { t.Fatal(out.String()) }
}
