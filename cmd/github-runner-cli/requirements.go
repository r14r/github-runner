package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type toolRequirement struct {
	Name    string
	Version string
	Source  string
}
type toolchainManifest struct {
	Repository string
	Revision   string
	Profile    string
	Image      string
	Workflows  []string
	Actions    []string
	Tools      []toolRequirement
	Warnings   []string
}

var workflowJob = regexp.MustCompile(`^  [A-Za-z0-9_.-]+:\s*(?:#.*)?$`)
var jobStepUses = regexp.MustCompile(`^\s+- uses:\s*([^\s#]+)`)
var stepWith = regexp.MustCompile(`^\s+with:\s*$`)
var fieldKey = regexp.MustCompile(`^\s+([A-Za-z][A-Za-z0-9_-]+):\s*(.*?)\s*$`)
var usesTool = map[string]string{
	"actions/setup-python":        "python",
	"actions/setup-java":          "java",
	"actions/setup-node":          "node",
	"actions/setup-go":            "go",
	"ruby/setup-ruby":             "ruby",
	"typst-community/setup-typst": "typst",
	"shivammathur/setup-php":      "php",
}
var versionField = map[string]string{
	"python": "python-version", "java": "java-version",
	"node": "node-version", "go": "go-version",
	"ruby": "ruby-version", "typst": "typst-version",
	"php": "php-version",
}
var runToolPatterns = map[string]*regexp.Regexp{
	"python":     regexp.MustCompile(`\b(python|python3|pytest|ruff|pip)\b`),
	"java":       regexp.MustCompile(`\b(java|javac|epubcheck)\b`),
	"typst":      regexp.MustCompile(`\btypst\b`),
	"just":       regexp.MustCompile(`\bjust\s+[-\w]+`),
	"ffmpeg":     regexp.MustCompile(`\bffmpeg\b`),
	"fontconfig": regexp.MustCompile(`\bfc-list\b`),
	"php":        regexp.MustCompile(`\bphp(?:8[.]2)?\b`),
	"node":       regexp.MustCompile(`\b(node|npm|npx)\b`),
	"go":         regexp.MustCompile(`\bgo\s+(?:test|build|vet|run|mod)\b`),
	"docker":     regexp.MustCompile(`\bdocker\s+(?:build|run|compose|info)\b`),
}
var supportedToolChecks = map[string]string{
	"python":     "command -v python3 >/dev/null",
	"java":       "command -v java >/dev/null",
	"typst":      "command -v typst >/dev/null",
	"just":       "command -v just >/dev/null",
	"ffmpeg":     "command -v ffmpeg >/dev/null",
	"fontconfig": "command -v fc-list >/dev/null",
	"php":        "command -v php >/dev/null",
	"node":       "command -v node >/dev/null",
	"go":         "command -v go >/dev/null",
	"docker":     "command -v docker >/dev/null",
	"ruby":       "command -v ruby >/dev/null",
}

func scalar(s string) string {
	data, _ := json.Marshal(s) // JSON string literals are valid YAML double-quoted scalars.
	return string(data)
}
func yamlManifest(m toolchainManifest) []byte {
	var b strings.Builder
	fmt.Fprintln(&b, "schema_version: 1")
	fmt.Fprintf(&b, "repository: %s\nrevision: %s\nprofile: %s\nimage: %s\n", scalar(m.Repository), scalar(m.Revision), scalar(m.Profile), scalar(m.Image))
	writeList := func(name string, values []string) {
		if len(values) == 0 {
			fmt.Fprintln(&b, name+": []")
			return
		}
		fmt.Fprintln(&b, name+":")
		for _, v := range values {
			fmt.Fprintln(&b, "  - "+scalar(v))
		}
	}
	writeList("workflows", m.Workflows)
	writeList("actions", m.Actions)
	if len(m.Tools) == 0 {
		fmt.Fprintln(&b, "tools: []")
	} else {
		fmt.Fprintln(&b, "tools:")
		for _, t := range m.Tools {
			fmt.Fprintf(&b, "  - name: %s\n    version: %s\n    source: %s\n", scalar(t.Name), scalar(t.Version), scalar(t.Source))
		}
	}
	writeList("warnings", m.Warnings)
	return []byte(b.String())
}
func trimYamlScalar(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, " #", 2)[0])
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}
func addTool(tools map[string]toolRequirement, name, version, source string) {
	if name == "" {
		return
	}
	if old, ok := tools[name]; ok {
		if old.Version != "" && version == "" {
			return
		}
		if old.Version == version && old.Source != "" {
			return
		}
		if old.Version != "" && version != "" && old.Version != version {
			// Preserve the first version. parseWorkflows emits a warning.
			return
		}
	}
	tools[name] = toolRequirement{Name: name, Version: version, Source: source}
}

// analyzeWorkflowText only inspects job steps. It never executes checkout code,
// workflows, scripts, or expressions, and does not claim arbitrary uses are
// pre-installed host dependencies.
func analyzeWorkflowText(path, data string, tools map[string]toolRequirement, actions map[string]bool, warnings *[]string) {
	activeAction, activeTool := "", ""
	withIndent := -1
	inJobs := false
	for _, line := range strings.Split(data, "\n") {
		raw := strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			inJobs = trimmed == "jobs:"
			activeAction = ""
			activeTool = ""
			withIndent = -1
			continue
		}
		if !inJobs {
			continue
		}
		if workflowJob.MatchString(raw) {
			activeAction = ""
			activeTool = ""
			withIndent = -1
			continue
		}
		if withIndent >= 0 && indent <= withIndent {
			withIndent = -1
		}
		if m := jobStepUses.FindStringSubmatch(raw); m != nil {
			action := trimYamlScalar(m[1])
			actions[action] = true
			activeAction = action
			activeTool = ""
			withIndent = -1
			prefix := strings.SplitN(action, "@", 2)[0]
			if tool, ok := usesTool[prefix]; ok {
				activeTool = tool
				addTool(tools, tool, "", path+": "+action)
			} else if strings.Contains(prefix, "setup-") {
				*warnings = append(*warnings, path+": unknown setup action "+action+" (manual review needed)")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && indent >= 4 {
			activeAction = ""
			activeTool = ""
			withIndent = -1
		}
		if stepWith.MatchString(raw) && activeTool != "" {
			withIndent = indent
			continue
		}
		if withIndent >= 0 && activeTool != "" && indent > withIndent {
			if m := fieldKey.FindStringSubmatch(raw); m != nil && m[1] == versionField[activeTool] {
				value := trimYamlScalar(m[2])
				if strings.Contains(value, "${{") || value == "" {
					*warnings = append(*warnings, path+": dynamic/empty "+m[1]+" for "+activeAction)
				} else {
					if old, ok := tools[activeTool]; ok && old.Version != "" && old.Version != value {
						*warnings = append(*warnings, path+": conflicting "+activeTool+" versions "+old.Version+" vs "+value)
					} else {
						addTool(tools, activeTool, value, path+": "+activeAction)
					}
				}
			}
			continue
		}
		// Commands inside multiline run scripts and one-line 'run:' steps.
		if strings.HasPrefix(trimmed, "run:") || indent >= 8 {
			for name, re := range runToolPatterns {
				if re.MatchString(trimmed) {
					addTool(tools, name, "", path+": run command")
				}
			}
		}
	}
}

// Local source reader used by tests and by the CLI after a read-only sparse
// clone. No workflow is executed; dotfiles and untrusted workflow code are data.
func inspectWorkflowDirectory(repo, root, revision, profile, image string) (toolchainManifest, error) {
	manifest := toolchainManifest{Repository: repo, Revision: revision, Profile: profile, Image: image}
	dir := filepath.Join(root, ".github", "workflows")
	files := []string{}
	for _, glob := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(dir, glob))
		if err != nil {
			return manifest, err
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return manifest, errors.New("repository has no .github/workflows/*.yml or *.yaml files")
	}
	actions := map[string]bool{}
	tools := map[string]toolRequirement{}
	for _, file := range files {
		info, err := os.Lstat(file)
		if err != nil {
			return manifest, err
		}
		if !info.Mode().IsRegular() {
			return manifest, fmt.Errorf("unsafe workflow file: %s", file)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return manifest, err
		}
		rel := ".github/workflows/" + filepath.Base(file)
		manifest.Workflows = append(manifest.Workflows, rel)
		analyzeWorkflowText(rel, string(data), tools, actions, &manifest.Warnings)
	}
	// The BookBuilder image profile defines additional preinstalled host tools
	// enforced by its pinned doctor. These are NOT inferred from a workflow's
	// arbitrary run scripts or falsely attributed to setup actions.
	if profile == "bookbuilder" {
		for _, tool := range []toolRequirement{
			{"just", "1.58.0", "runner profile: bookbuilder"},
			{"ffmpeg", "", "runner profile: bookbuilder"},
			{"fontconfig", "", "runner profile: bookbuilder"},
			{"php", "8.2", "runner profile: bookbuilder"},
			{"java", "21", "runner profile: bookbuilder"},
			{"typst", "0.15.1", "runner profile: bookbuilder"},
			{"python", "3.12", "runner profile: bookbuilder"},
		} {
			if old, ok := tools[tool.Name]; ok && old.Version != "" && tool.Version != "" && old.Version != tool.Version {
				manifest.Warnings = append(manifest.Warnings, "workflow requests "+tool.Name+" "+old.Version+" but bookbuilder profile pins "+tool.Version)
			}
			tools[tool.Name] = tool
		}
	}
	for a := range actions {
		manifest.Actions = append(manifest.Actions, a)
	}
	sort.Strings(manifest.Actions)
	for _, t := range tools {
		manifest.Tools = append(manifest.Tools, t)
	}
	sort.Slice(manifest.Tools, func(i, j int) bool { return manifest.Tools[i].Name < manifest.Tools[j].Name })
	sort.Strings(manifest.Warnings)
	return manifest, nil
}
func discoverRequirements(r dockerRunner) (toolchainManifest, error) {
	var empty toolchainManifest
	if _, err := exec.LookPath("gh"); err != nil {
		return empty, fmt.Errorf("GitHub CLI is required for requirements: %w", err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return empty, fmt.Errorf("Git is required for requirements: %w", err)
	}
	dir, err := os.MkdirTemp("", "ghr-requirements-")
	if err != nil {
		return empty, err
	}
	defer os.RemoveAll(dir)
	if err := commandRun("", "gh", "repo", "clone", r.Repo, dir, "--", "--depth=1", "--filter=blob:none", "--sparse", "--quiet"); err != nil {
		return empty, fmt.Errorf("cannot clone %s: %w", r.Repo, err)
	}
	if err := commandRun(dir, "git", "sparse-checkout", "set", ".github/workflows"); err != nil {
		return empty, err
	}
	rev, err := commandOutput(dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return empty, err
	}
	return inspectWorkflowDirectory(r.Repo, dir, rev, r.Profile, r.Image)
}
func parseRequirementsArgs(args []string) (string, string, error) {
	repo, rest, err := requireRepo(args)
	if err != nil {
		return "", "", err
	}
	output := ""
	for i := 0; i < len(rest); i++ {
		if rest[i] != "--output" {
			return "", "", fmt.Errorf("requirements: unknown option %q", rest[i])
		}
		if i+1 >= len(rest) || strings.HasPrefix(rest[i+1], "-") {
			return "", "", errors.New("--output requires a YAML file path")
		}
		if output != "" {
			return "", "", errors.New("--output specified multiple times")
		}
		i++
		output = rest[i]
		if output == "-" {
			return "", "", errors.New("use no --output to print to stdout")
		}
	}
	return repo, output, nil
}
func saveManifest(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular manifest output: %s", path)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(path)
	if _, err := os.Stat(parent); err != nil {
		return fmt.Errorf("output directory missing: %w", err)
	}
	temp, err := os.CreateTemp(parent, ".ghr-toolchain-*.yaml")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func requirementsCommand(repo string, output string, assets string) error {
	r, err := newDockerRunner(repo, assets)
	if err != nil {
		return err
	}
	m, err := discoverRequirements(r)
	if err != nil {
		return err
	}
	data := yamlManifest(m)
	if output == "" {
		fmt.Print(string(data))
		return nil
	}
	if err := saveManifest(output, data); err != nil {
		return err
	}
	fmt.Printf("Analyzed %d workflows and %d tools; wrote %s (revision %s)\n", len(m.Workflows), len(m.Tools), output, m.Revision)
	if len(m.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: %d items need manual review; see warnings in manifest\n", len(m.Warnings))
	}
	return nil
}
func validateProfile(m toolchainManifest) error {
	knownVersions := map[string]map[string]string{
		"runner-base": {"python": "3.12"},
		"bookbuilder": {"python": "3.12", "java": "21", "typst": "0.15.1", "php": "8.2", "just": "1.58.0"},
	}
	available := map[string]map[string]bool{
		"runner-base": {"python": true, "docker": true, "node": true},
		"bookbuilder": {"python": true, "docker": true, "node": true, "java": true, "typst": true, "php": true, "just": true, "ffmpeg": true, "fontconfig": true},
	}
	for _, t := range m.Tools {
		if !available[m.Profile][t.Name] && strings.Contains(t.Source, "setup-") {
			// Official setup actions provision their own runtime at job time.
			continue
		}
		if !available[m.Profile][t.Name] {
			return fmt.Errorf("runner profile %s does not include required tool %s (%s); select an appropriate RUNNER_PROFILE or extend the Dockerfile", m.Profile, t.Name, t.Source)
		}
		if pin := knownVersions[m.Profile][t.Name]; pin != "" && t.Version != "" && pin != t.Version {
			return fmt.Errorf("runner profile %s provides %s %s, but workflows require %s", m.Profile, t.Name, pin, t.Version)
		}
	}
	if len(m.Warnings) > 0 {
		return fmt.Errorf("%d unresolved workflow requirements; inspect 'requirements --repo %s' before building", len(m.Warnings), m.Repository)
	}
	return nil
}

// doctor checks installed image capabilities WITHOUT requiring a registered
// runner, stopping an active service, or modifying GitHub registrations.
func toolchainDoctor(r dockerRunner) error {
	m, err := discoverRequirements(r)
	if err != nil {
		return err
	}
	if err = validateProfile(m); err != nil {
		return err
	}
	if err = r.selectPlatform(); err != nil {
		return err
	}
	if !r.exists("image", r.Image) {
		return fmt.Errorf("image %s missing; run build --repo %s", r.Image, r.Repo)
	}
	for _, t := range m.Tools {
		if strings.Contains(t.Source, "setup-") && !map[string]bool{
			"python": true, "java": m.Profile == "bookbuilder", "typst": m.Profile == "bookbuilder",
			"php": m.Profile == "bookbuilder", "just": m.Profile == "bookbuilder",
			"ffmpeg": m.Profile == "bookbuilder", "fontconfig": m.Profile == "bookbuilder",
			"docker": true, "node": false, "go": false, "ruby": false,
		}[t.Name] {
			fmt.Printf("ACTION: %s %s (installed by workflow step %s)\n", t.Name, t.Version, t.Source)
			continue
		}
		check, ok := supportedToolChecks[t.Name]
		if !ok {
			return fmt.Errorf("no doctor check defined for %s", t.Name)
		}
		// Version requirements are checked for the supported, pinned tools.
		switch t.Name {
		case "python":
			if t.Version != "" {
				check += " && python3 --version 2>&1 | grep -F " + shellSingleQuote(t.Version)
			}
		case "java":
			if t.Version != "" {
				check += " && java -version 2>&1 | grep -F " + shellSingleQuote(t.Version)
			}
		case "php":
			if t.Version != "" {
				check += " && php -v | grep -F " + shellSingleQuote(t.Version)
			}
		case "typst":
			if t.Version != "" {
				check += " && typst --version | grep -F " + shellSingleQuote(t.Version)
			}
		case "just":
			if t.Version != "" {
				check += " && just --version | grep -F " + shellSingleQuote(t.Version)
			}
		case "node":
			if t.Version != "" {
				check += " && node --version | grep -F " + shellSingleQuote(t.Version)
			}
		case "go":
			if t.Version != "" {
				check += " && go version | grep -F " + shellSingleQuote(t.Version)
			}
		case "ruby":
			if t.Version != "" {
				check += " && ruby --version | grep -F " + shellSingleQuote(t.Version)
			}
		}
		if err := commandRun("", "docker", "run", "--rm", "--platform", dockerPlatform, "--entrypoint", "bash", r.Image, "-euc", check); err != nil {
			return fmt.Errorf("FAIL: %s %s (%s): %w", t.Name, t.Version, t.Source, err)
		}
		fmt.Printf("OK: %s %s\n", t.Name, t.Version)
	}
	fmt.Printf("Toolchain doctor passed: %s (%d checked tools)\n", r.Image, len(m.Tools))
	return nil
}
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
func buildCommand(r dockerRunner) error {
	m, err := discoverRequirements(r)
	if err != nil {
		return err
	}
	if err := validateProfile(m); err != nil {
		return err
	}
	fmt.Printf("Workflow analysis: %d workflows, %d actions, %d required host tools; profile=%s\n", len(m.Workflows), len(m.Actions), len(m.Tools), m.Profile)
	if err := r.download(); err != nil {
		return err
	}
	return toolchainDoctor(r)
}
