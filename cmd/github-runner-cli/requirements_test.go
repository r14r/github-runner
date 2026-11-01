package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequirementsAnalyzeMultipleWorkflows(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"ci.yml": `name: CI
on: [push]
jobs:
  build:
    runs-on: [self-hosted, Linux, X64]
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-python@v7
        with:
          python-version: "3.12"
      - uses: actions/setup-java@v6
        with:
          java-version: '21'
          distribution: temurin
      - name: Install Typst
        uses: typst-community/setup-typst@v5.3.0
        with:
          typst-version: '0.15.1'
      - name: build
        run: |
          just ci
          ffmpeg -version
          php -v
          typst --version
`,
		"pages.yaml": `jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: ruby/setup-ruby@v1
        with:
          ruby-version: '3.3'
      - name: Build
        run: |
          bundle exec jekyll build
`,
	}
	for name, content := range cases {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := inspectWorkflowDirectory("example-org/demo-app", root, "abcdef", "toolchain", "test:tag")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Workflows) != 2 {
		t.Errorf("workflows: %#v", m.Workflows)
	}
	for _, want := range []string{"actions/checkout@v7", "actions/setup-java@v6", "ruby/setup-ruby@v1", "typst-community/setup-typst@v5.3.0"} {
		if !containsString(m.Actions, want) {
			t.Errorf("action %s missing: %v", want, m.Actions)
		}
	}
	versions := map[string]string{}
	for _, tool := range m.Tools {
		versions[tool.Name] = tool.Version
	}
	for name, want := range map[string]string{"python": "3.12", "java": "21", "typst": "0.15.1", "php": "", "just": "", "ruby": "3.3"} {
		if versions[name] != want {
			t.Errorf("%s version: got %q expected %q", name, versions[name], want)
		}
	}
	if err := validateProfile(m); err != nil {
		t.Errorf("Setup actions should provision their own runtimes: %v", err)
	}
	manifest := string(yamlManifest(m))
	for _, want := range []string{
		"schema_version: 1", `repository: "example-org/demo-app"`,
		`revision: "abcdef"`, `name: "ruby"`,
		`version: "3.3"`, `"actions/checkout@v7"`,
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest missing %q", want)
		}
	}
}
func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func TestRequirementsOutputAndMissingFlags(t *testing.T) {
	for _, args := range [][]string{nil, {"--output", "toolchain.yaml"}, {"--repo"}, {"--repo", "example-org/demo-app", "--output"}, {"--repo", "example-org/demo-app", "--unknown"}, {"--repo", "example-org/demo-app", "--output", "a", "--output", "b"}} {
		if _, _, err := parseRequirementsArgs(args); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	repo, out, err := parseRequirementsArgs([]string{"--output", "toolchain.yaml", "--repository", "example-org/demo-app"})
	if err != nil || repo != "example-org/demo-app" || out != "toolchain.yaml" {
		t.Fatalf("%s %s %v", repo, out, err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "toolchain.yaml")
	if err := saveManifest(file, []byte("schema_version: 1\n")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "schema_version: 1\n" {
		t.Fatal(err, string(b))
	}
	if err := os.Symlink(file, filepath.Join(dir, "link.yaml")); err == nil {
		if err := saveManifest(filepath.Join(dir, "link.yaml"), []byte("unsafe")); err == nil {
			t.Fatal("overwrote symlink")
		}
	}
}
func TestRequirementsRejectsDynamicVersionAndUnknownAction(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: custom/setup-exotic@v3
      - uses: actions/setup-python@v7
        with:
          python-version: ${{ matrix.python }}
`
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := inspectWorkflowDirectory("example-org/test", root, "abc", "toolchain", "runner:test")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 2 {
		t.Fatalf("warnings: %v", m.Warnings)
	}
	if err := validateProfile(m); err == nil {
		t.Fatal("unresolved workflow requirements must block build")
	}
}
func TestRequirementsNoWorkflows(t *testing.T) {
	_, err := inspectWorkflowDirectory("example-org/example", t.TempDir(), "abc", "runner-base", "test")
	if err == nil {
		t.Fatal("missing workflows unexpectedly passed")
	}
}
func TestRequirementsProfileRejectsUnsupportedHostTool(t *testing.T) {
	m := toolchainManifest{Repository: "example-org/test", Profile: "runner-base", Tools: []toolRequirement{
		{Name: "java", Version: "21", Source: ".github/workflows/test.yml: run command"},
	}}
	if err := validateProfile(m); err == nil {
		t.Fatal("runner-base should reject Java 21")
	}
}
func TestNewRequirementCommandsHaveHelpWithoutRepo(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_CLI_ENV", filepath.Join(t.TempDir(), "missing-assets"))
	for _, command := range []string{"requirements", "build", "doctor"} {
		output, err := captureCLI(t, command, "--help")
		if err != nil || !strings.Contains(output, "USAGE:") || !strings.Contains(output, "EXAMPLES:") {
			t.Errorf("%s help failed: %q %v", command, output, err)
		}
	}
}
func TestReadonlyCIRecipe(t *testing.T) {
	file := filepath.Join("..", "..", "justfile")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "ci: bootstrap fmt lint test") {
		t.Fatal("ci still mutates source formatting")
	}
	if !strings.Contains(s, "ci: bootstrap fmt-check lint test") {
		t.Fatal("ci must check formatting, not rewrite it")
	}
}

func TestGenericToolchainPlannerAndActionProvisioning(t *testing.T) {
    m:=toolchainManifest{Repository:"example-org/demo-app",Profile:"toolchain",
        Tools:[]toolRequirement{
            {Name:"ffmpeg",Source:".github/workflows/ci.yml: run command"},
            {Name:"fontconfig",Source:".github/workflows/ci.yml: run command"},
            {Name:"just",Version:"1.58.0",Source:".github/workflows/ci.yml: run command"},
            {Name:"java",Version:"21",Source:".github/workflows/ci.yml: actions/setup-java@v6"},
            {Name:"ruby",Version:"3.3",Source:".github/workflows/pages.yml: ruby/setup-ruby@v1"},
            {Name:"typst",Version:"0.15.1",Source:".github/workflows/pdf.yml: typst-community/setup-typst@v5"},
        }}
    args,err:=toolchainBuildArgs(m)
    if err!=nil{t.Fatal(err)}
    joined:=strings.Join(args," ")
    for _,want:=range []string{"EXTRA_APT_PACKAGES=ffmpeg fontconfig","INSTALL_JUST=1"}{
        if !strings.Contains(joined,want){t.Errorf("missing %q from %q",want,joined)}
    }
    for _,no:=range []string{"openjdk","INSTALL_TYPST=1","ruby"}{
        if strings.Contains(joined,no){t.Errorf("action-managed package %q incorrectly preinstalled: %q",no,joined)}
    }
}
func TestToolchainPlannerRefusesUnsupportedVersion(t *testing.T) {
    m:=toolchainManifest{Repository:"example-org/demo-app",Profile:"toolchain",
        Tools:[]toolRequirement{{Name:"php",Version:"8.2",Source:"workflow run command"}}}
    if _,err:=toolchainBuildArgs(m);err==nil{t.Fatal("unsupported PHP version accepted")}
    m.Tools[0].Source=".github/workflows/ci.yml: shivammathur/setup-php@v2"
    if _,err:=toolchainBuildArgs(m);err!=nil{t.Fatal("official setup action should be allowed:",err)}
}
func TestDoctorStatusLineFormatting(t *testing.T){
    for _,tc:=range []struct{name,version,status,want string}{
        {"ffmpeg","","OK","- ffmpeg:"},
        {"java","21","OK","- java (21):"},
        {"ruby","3.3","WORKFLOW ACTION","- ruby (3.3):"},
    }{
        line:=toolStatusLine(tc.name,tc.version,tc.status)
        if !strings.Contains(line,tc.want)||!strings.HasSuffix(line,tc.status){
            t.Errorf("unexpected doctor row %q",line)
        }
    }
}
func TestQuietGitCapturesFailureOnly(t *testing.T){
    if err:=capturedGit("","sh","-c","printf noisy-git-output >&2");err!=nil{t.Fatal(err)}
    err:=capturedGit("","sh","-c","printf git-failed >&2; exit 12")
    if err==nil||!strings.Contains(err.Error(),"git-failed"){t.Errorf("expected diagnostic error, got %v",err)}
}
