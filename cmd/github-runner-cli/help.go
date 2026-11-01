package main

import (
	"fmt"
	"os"
	"strings"
)

// Help is handled before runtime asset discovery, flag validation, or any
// Docker/GitHub command. It is deliberately safe without an installed runner.
type commandDoc struct {
	Summary     string
	Description string
	Usage       []string
	Options     []string
	Values      []string
	Examples    []string
	Notes       []string
}

const repositoryFlagHelp = "--repo, --repository OWNER/REPO  Required GitHub repository; accepts --repo=OWNER/REPO and HTTPS GitHub URL."

var commandDocs = map[string]commandDoc{
	"help": {
		Summary:     "Show the overview or detailed documentation for a command.",
		Description: "Prints command syntax, what the command does, accepted parameters and values, examples, and safety notes. Never connects to GitHub or Docker.",
		Usage:       []string{"ghr help", "ghr help <command>", "ghr help runner <subcommand>", "ghr <command> --help"},
		Options:     []string{"<command>              Optional command name (e.g. status, install, list-installed).", "<subcommand>           Required for detailed runner subcommand documentation.", "-h, --help             Show help (accepted before any required option)."},
		Examples:    []string{"ghr help", "ghr help status", "ghr status --help", "ghr help runner setup", "ghr runner setup --help"},
	},
	"version": {
		Summary:     "Show the version embedded in the installed Go binary.",
		Description: "Prints the CLI application version; it does not check Docker or GitHub. The GitHub Actions runner version is managed separately.",
		Usage:       []string{"ghr version"},
		Options:     []string{"-h, --help             Show this page."},
		Examples:    []string{"ghr version"},
	},
	"requirements": {
		Summary:     "Analyze repository workflows and report required toolchains.",
		Description: "Performs a read-only sparse checkout of GitHub Actions workflow files. Detects uses actions, versions from recognized setup actions, and supported run commands. Does not execute repository workflows.",
		Usage:       []string{"ghr requirements --repo OWNER/REPO [--output PATH]"},
		Options:     []string{repositoryFlagHelp, "--output PATH           Write the toolchain manifest as YAML (otherwise print to stdout).", "-h, --help             Show this page."},
		Values:      []string{"--repo: GitHub OWNER/REPO.", "--output: local YAML file path, e.g. toolchain.yaml."},
		Examples:    []string{"ghr requirements --repo example-org/demo-app", "ghr requirements --repo example-org/demo-app --output toolchain.yaml"},
		Notes:       []string{"Unknown or dynamic requirements are reported, not silently installed.", "Workflow-provisioned tools are identified separately from image-installed tools."},
	},
	"build": {
		Summary:     "Prepare the runner Docker image according to the analyzed workflow requirements.",
		Description: "Inspects the repository workflows, validates the selected image profile against detectable requirements, builds it, and verifies the resulting toolchain. A recognized profile is required; arbitrary dependencies are not auto-installed.",
		Usage:       []string{"ghr build --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"RUNNER_BACKEND: auto, docker.", "RUNNER_PROFILE: auto, generic, toolchain, runner-base."},
		Examples:    []string{"ghr requirements --repo example-org/demo-app --output toolchain.yaml", "ghr build --repo example-org/demo-app", "ghr doctor --repo example-org/demo-app"},
		Notes:       []string{"The build prepares an image and registration volume but does not register/start a GitHub runner.", "Unrecognized required tools fail closed; extend the Dockerfile/profile first."},
	},
	"doctor": {
		Summary:     "Verify the prepared Docker image satisfies workflow requirements.",
		Description: "Rechecks current workflow requirements, locates the local Docker image, and verifies the supported tools and pinned versions. No GitHub registration or running runner is needed.",
		Usage:       []string{"ghr doctor --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"RUNNER_PROFILE: auto, generic, toolchain, runner-base.", "RUNNER_IMAGE: optional Docker image override."},
		Examples:    []string{"ghr build --repo example-org/demo-app", "ghr doctor --repo example-org/demo-app", "ghr start --repo example-org/demo-app"},
		Notes:       []string{"Doctor does not install missing tools.", "It checks the prepared image; start still requires a registered runner."},
	},
	"install": {
		Summary:     "Build the runner image, register a repository runner and create its Docker container.",
		Description: "Runs the Docker preflight, derives safe provisioning arguments from workflow requirements, builds a repository-isolated toolchain image, registers the runner and prepares its container.",
		Usage:       []string{"ghr install --repo OWNER/REPO [--allow-public] [--name NAME]"},
		Options:     []string{repositoryFlagHelp, "--allow-public          Explicitly acknowledge running a trusted workflow from a PUBLIC repository.", "--name NAME             Optional runner registration name; ASCII letters/digits/underscore/dot/hyphen.", "-h, --help             Show this page."},
		Values:      []string{"--repo: OWNER/REPO, e.g. example-org/demo-app.", "RUNNER_BACKEND: docker or auto (Colima on Monterey).", "RUNNER_PROFILE: auto (default), generic, toolchain, runner-base.", "RUNNER_DOCKER_SOCKET: 0 (default) or 1 (privileged opt-in)."},
		Examples:    []string{"ghr install --repo example-org/another-app", "ghr install --repository example-org/demo-app --allow-public", "ghr install --repo example-org/demo-app --allow-public --name demo-runner", "ghr start --repo example-org/demo-app"},
		Notes:       []string{"A public repository requires --allow-public.", "An already configured registration volume is preserved.", "Stop active workflows before replacing any existing container."},
	},
	"start": {
		Summary:     "Start an installed repository runner container.",
		Description: "Verifies the persisted registration and image. Starts an existing stopped Docker container, or creates it if missing. Does not install tools or re-register the runner.",
		Usage:       []string{"ghr start --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Examples:    []string{"ghr start --repo example-org/demo-app", "ghr status --repo example-org/demo-app"},
		Notes:       []string{"Install the runner first.", "If the image changed, stop and refresh the container before starting."},
	},
	"stop": {
		Summary:     "Stop an installed repository runner container.",
		Description: "Stops the Docker container without deleting its GitHub registration or persistent workspace volume.",
		Usage:       []string{"ghr stop --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Examples:    []string{"ghr status --repo example-org/demo-app", "ghr stop --repo example-org/demo-app"},
		Notes:       []string{"Stopping a busy runner interrupts an active GitHub Actions job; check status first."},
	},
	"status": {
		Summary:     "Display container state and GitHub online/busy state for one repository runner.",
		Description: "Checks local Docker container state, whether its image is outdated, and (where available) its corresponding GitHub runner registration. This command is read-only.",
		Usage:       []string{"ghr status --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"Container status: created, running, exited, paused, etc.", "GitHub status: online or offline; busy=true or false."},
		Examples:    []string{"ghr status --repo example-org/demo-app", "ghr status --repository example-org/another-app", "ghr status --help"},
		Notes:       []string{"A configured runner and Docker daemon are required to retrieve status.", "Help works without --repo, Docker, or a configured registration."},
	},
	"monitor": {
		Summary:     "Continuously display runner status, recent GitHub runs and container logs.",
		Description: "Periodically refreshes local/GitHub runner status, recent workflow runs, and the tail of the Docker logs until interrupted.",
		Usage:       []string{"ghr monitor --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"RUNNER_MONITOR_SECONDS: integer >= 2; defaults to 10 seconds."},
		Examples:    []string{"ghr monitor --repo example-org/demo-app", "RUNNER_MONITOR_SECONDS=5 ghr monitor --repo example-org/demo-app"},
		Notes:       []string{"Ctrl+C stops monitoring, not the runner.", "Detailed step logs belong to GitHub Actions: gh run view RUN_ID --repo OWNER/REPO --log."},
	},
	"refresh": {
		Summary:     "Replace a STOPPED runner container with the currently built image.",
		Description: "Recreates the container from the chosen image while retaining its existing registration/workspace Docker volume.",
		Usage:       []string{"ghr refresh --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"RUNNER_DOCKER_SOCKET: 1 explicitly preserves an existing privileged socket mount."},
		Examples:    []string{"ghr status --repo example-org/demo-app", "ghr stop --repo example-org/demo-app", "ghr runner download --repo example-org/demo-app", "ghr refresh --repo example-org/demo-app", "ghr start --repo example-org/demo-app"},
		Notes:       []string{"Refuses to replace a running container. Wait for busy=false before stopping.", "Does not delete or re-register the existing runner."},
	},
	"list-installed": {
		Summary:     "List managed local Docker runners; optionally include a repository's GitHub registrations.",
		Description: "Without a repository, discovers local runner containers and registration volumes in the current Docker context. With --repo, filters local installations and queries the selected repository's runner registrations, including other hosts.",
		Usage:       []string{"ghr list-installed [--repo OWNER/REPO]", "ghr list-installed --all"},
		Options:     []string{"--repo, --repository OWNER/REPO  Optional repository filter; with GitHub API registration details.", "--all                   Explicit alias for listing all LOCAL installations (default without --repo).", "-h, --help             Show this page."},
		Examples:    []string{"ghr list-installed", "ghr list-installed --all", "ghr list-installed --repo example-org/demo-app", "ghr list-installed --repository example-org/another-app"},
		Notes:       []string{"Without --repo this does NOT enumerate all runners registered in every GitHub repository.", "Read-only; the --repo form requires GitHub CLI API access."},
	},
	"uninstall": {
		Summary:     "Unregister and remove selected local Docker runners.",
		Description: "Validates each local runner's persisted identity against GitHub and refuses busy runners. Unregisters the matched GitHub runner, stops/removes its container and, only when requested, deletes its persistent volume. Bulk operations preflight all candidates.",
		Usage:       []string{"ghr uninstall --repo OWNER/REPO [--dry-run] [--delete-data]", "ghr uninstall --all --yes [--delete-data]", "ghr uninstall --all --dry-run [--delete-data]"},
		Options:     []string{"--repo, --repository OWNER/REPO  Select exactly one locally installed repository runner.", "--all                   Select ALL locally managed runners in this Docker context; exclusive with --repo.", "--yes                   Required for --all unless --dry-run is set.", "--dry-run               Preview the removal plan; do not change containers, GitHub, or volumes.", "--delete-data           Also delete the persistent Docker registration/workspace volume (irreversible).", "-h, --help             Show this page."},
		Examples:    []string{"ghr uninstall --repo example-org/demo-app --dry-run", "ghr uninstall --repo example-org/demo-app", "ghr uninstall --repo example-org/demo-app --delete-data", "ghr uninstall --all --dry-run", "ghr uninstall --all --yes --delete-data"},
		Notes:       []string{"Does not remove Docker images or unrelated Docker volumes.", "Existing workspace/registration volume is kept by default; --delete-data permanently deletes it.", "No --all deletion without --yes; busy and unidentifiable registrations are refused.", "Does not uninstall macOS launchd or Ubuntu systemd host services."},
	},
	"self-uninstall": {
		Summary:     "Delete the installed CLI binary and its adjacent runtime assets.",
		Description: "Removes only the installed github-runner-cli binary and <INSTALL>/env/github-runner-cli. Requires a real, non-symlink installation; refuses temporary go run paths and GITHUB_RUNNER_CLI_ENV overrides.",
		Usage:       []string{"ghr self-uninstall --yes"},
		Options:     []string{"--yes                   Required confirmation; this operation is irreversible.", "-h, --help             Show this page."},
		Examples:    []string{"ghr self-uninstall --help", "ghr self-uninstall --yes"},
		Notes:       []string{"Does NOT stop/unregister runners or remove Docker containers, images or volumes.", "Run from an installed binary with sufficient filesystem permissions."},
	},
	"runner": {
		Summary:     "Advanced repository-specific runner operations.",
		Description: "Manage individual lifecycle stages and workflow-routing pull requests. For a new runner prefer the top-level install command.",
		Usage:       []string{"ghr runner <subcommand> --repo OWNER/REPO [options]", "ghr runner docker-doctor", "ghr help runner <subcommand>"},
		Options:     []string{"Subcommands: download, register, configure, setup, pr, install-service, start, stop, status, monitor, foreground, replace-container, uninstall-service, docker-doctor.", "--repo, --repository OWNER/REPO  Required for all subcommands except docker-doctor.", "-h, --help             Show the runner overview or subcommand help."},
		Examples:    []string{"ghr runner docker-doctor", "ghr runner download --repo example-org/demo-app", "ghr runner register --repo example-org/demo-app --allow-public", "ghr runner setup --repo example-org/demo-app --workflow .github/workflows/ci.yml --job build --allow-public", "ghr runner status --help"},
	},
	"runner docker-doctor": {
		Summary:     "Check the Linux x64 Docker environment and Ubuntu APT integrity.",
		Description: "Checks Docker daemon/architecture/context, image storage, writable container free space (minimum 2 GiB), and signed Ubuntu package indexes. Useful before image builds, especially with Colima.",
		Usage:       []string{"ghr runner docker-doctor"},
		Options:     []string{"-h, --help             Show this page. No --repo is needed."},
		Values:      []string{"Supported Docker architecture: amd64/x86_64.", "Container writable free space: at least 2 GiB."},
		Examples:    []string{"docker context use colima", "ghr runner docker-doctor"},
		Notes:       []string{"Does not change apt signature settings or modify runner registrations."},
	},
	"runner download": {
		Summary:     "Build the selected repository runner Docker image and prepare its volume.",
		Description: "Analyzes repository workflows, builds the generic runner image and installs allowlisted tools in an isolated image for this repository. Does not register or start a runner.",
		Usage:       []string{"ghr runner download --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"RUNNER_PROFILE: auto, generic, toolchain, runner-base.", "RUNNER_IMAGE: optional explicit image tag override."},
		Examples:    []string{"ghr runner download --repo example-org/demo-app", "ghr runner register --repo example-org/demo-app --allow-public"},
	},
	"runner register": {
		Summary:     "Register a repository runner with GitHub using a fresh registration token.",
		Description: "Checks repository visibility and existing registration, obtains a short-lived token through gh, and configures the runner. Does not create/start a service container.",
		Usage:       []string{"ghr runner register --repo OWNER/REPO [--allow-public] [--name NAME]"},
		Options:     []string{repositoryFlagHelp, "--allow-public          Required for public repositories.", "--name NAME             Optional ASCII runner name (letters, numbers, underscore, dot, hyphen).", "-h, --help             Show this page."},
		Examples:    []string{"ghr runner download --repo example-org/demo-app", "ghr runner register --repo example-org/demo-app --allow-public"},
		Notes:       []string{"Existing runner registrations are not overwritten.", "The registration token is passed on standard input, not command arguments."},
	},
	"runner setup": {
		Summary:     "Open a GitHub pull request routing selected workflow jobs to the repository runner.",
		Description: "Clones the repository, validates and rewrites job-level runs-on selectors to the repository runner label, pushes a branch, and opens a PR. Merges only when explicitly requested and CI checks pass.",
		Usage:       []string{"ghr runner setup --repo OWNER/REPO [--workflow PATH] [--job ID] [--base BRANCH] [--allow-public] [--allow-cross-os] [--merge|--no-merge]"},
		Options:     []string{repositoryFlagHelp, "--workflow PATH         Workflow file relative to the repository, e.g. .github/workflows/ci.yml; repeatable.", "--job ID                Job identifier to change; repeatable. If omitted, selects supported jobs.", "--base BRANCH           PR base branch; defaults to repository default branch.", "--allow-public          Required for a public repository.", "--allow-cross-os        Permit macOS/Linux changes to runs-on; review carefully.", "--merge                 Watch required checks and squash-merge PR when successful.", "--no-merge              Leave PR open for review (default).", "-h, --help             Show this page."},
		Values:      []string{"--repo: OWNER/REPO.", "--workflow: .github/workflows/*.yml or *.yaml.", "--job: YAML job ID, e.g. build, test, python.", "--base: an existing Git branch name."},
		Examples:    []string{"ghr runner setup --repo example-org/demo-app --workflow .github/workflows/ci.yml --job build --allow-public", "ghr runner setup --repository example-org/demo-app --workflow .github/workflows/ci.yml --job test --allow-public --merge"},
		Notes:       []string{"Requires a registered runner and authenticated gh/git.", "Dynamic/multiline runs-on configurations are refused rather than guessed.", "Use --allow-cross-os only after reviewing the execution environment."},
	},
	"runner install-service": {
		Summary:     "Create the Docker service container for an already registered runner.",
		Description: "Creates the runner's restartable Docker container from its configured image and persistent volume, unless already present. Does not start it.",
		Usage:       []string{"ghr runner install-service --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Values:      []string{"RUNNER_DOCKER_SOCKET: 0 (default), 1 (privileged socket mount)."},
		Examples:    []string{"ghr runner install-service --repo example-org/demo-app", "ghr start --repo example-org/demo-app"},
	},
	"runner foreground": {
		Summary:     "Attach to the runner container's live process in the terminal.",
		Description: "Creates a missing service container if needed and starts it attached to your terminal; unlike monitor, this attaches to the container process.",
		Usage:       []string{"ghr runner foreground --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Examples:    []string{"ghr runner foreground --repo example-org/demo-app"},
		Notes:       []string{"This is not a read-only monitoring command. Use 'ghr monitor' for passive observation."},
	},
	"runner replace-container": {
		Summary:     "Replace a stopped service container with the new runner image.",
		Description: "The advanced equivalent of top-level refresh. Keeps the repository's persistent registration/workspace volume.",
		Usage:       []string{"ghr runner replace-container --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Examples:    []string{"ghr stop --repo example-org/demo-app", "ghr runner download --repo example-org/demo-app", "ghr runner replace-container --repo example-org/demo-app"},
		Notes:       []string{"Will not replace a running container.", "Does not delete the runner's registration volume."},
	},
	"runner uninstall-service": {
		Summary:     "Remove the runner service container but keep its registration.",
		Description: "Removes only the selected repository runner container. It does not unregister the runner on GitHub or delete the volume.",
		Usage:       []string{"ghr runner uninstall-service --repo OWNER/REPO"},
		Options:     []string{repositoryFlagHelp, "-h, --help             Show this page."},
		Examples:    []string{"ghr runner uninstall-service --repo example-org/demo-app", "ghr uninstall --repo example-org/demo-app --dry-run"},
		Notes:       []string{"May forcibly stop a running container; verify the runner is idle first.", "For complete removal, prefer top-level uninstall with --dry-run."},
	},
}

// Alias docs share the exact behavior of their preferred equivalent.
func init() {
	for alias, canonical := range map[string]string{
		"runner configure": "runner register",
		"runner pr":        "runner setup",
		"runner start":     "start",
		"runner stop":      "stop",
		"runner status":    "status",
		"runner monitor":   "monitor",
	} {
		doc := commandDocs[canonical]
		doc.Usage = append([]string(nil), doc.Usage...)
		for i := range doc.Usage {
			doc.Usage[i] = strings.Replace(doc.Usage[i], "ghr "+canonical, "ghr "+alias, 1)
		}
		if !strings.HasPrefix(alias, "runner ") || strings.HasPrefix(canonical, "runner ") {
			doc.Notes = append(doc.Notes, "Alias of: ghr "+canonical)
		} else {
			doc.Notes = append(doc.Notes, "Advanced alias for top-level: ghr "+canonical)
		}
		commandDocs[alias] = doc
	}

}

// maybeShowHelp interprets help requests before any other parsing, including
// the required repository flag, asset discovery, and side-effecting operations.
func maybeShowHelp(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	first := strings.ToLower(args[0])
	switch first {
	case "help":
		return true, showCommandHelp(args[1:])
	case "-h", "--help":
		return true, showCommandHelp(args[1:])
	}
	if first == "runner" {
		if len(args) > 1 && strings.EqualFold(args[1], "help") {
			return true, showCommandHelp(append([]string{first}, args[2:]...))
		}
	}
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			if (first == "runner") && len(args) > 2 && args[1] != "--help" && args[1] != "-h" {
				return true, showCommandHelp([]string{first, args[1]})
			}
			return true, showCommandHelp([]string{first})
		}
	}
	return false, nil
}

func showCommandHelp(path []string) error {
	filtered := make([]string, 0, len(path))
	for _, a := range path {
		if a == "-h" || a == "--help" {
			continue
		}
		filtered = append(filtered, strings.ToLower(a))
	}
	if len(filtered) == 0 {
		fmt.Fprint(os.Stdout, globalHelp)
		return nil
	}
	if len(filtered) > 2 {
		return fmt.Errorf("unknown help topic %q; use 'ghr help' for available commands", strings.Join(filtered, " "))
	}
	key := strings.Join(filtered, " ")
	doc, ok := commandDocs[key]
	if !ok {
		return fmt.Errorf("unknown help topic %q; use 'ghr help' for available commands", key)
	}
	fmt.Printf("ghr %s — %s\n\n%s\n", key, doc.Summary, doc.Description)
	printHelpSection("USAGE", doc.Usage)
	printHelpSection("OPTIONS", doc.Options)
	printHelpSection("VALUES", doc.Values)
	printHelpSection("EXAMPLES", doc.Examples)
	printHelpSection("NOTES", doc.Notes)
	fmt.Print("\nTip: 'ghr help' lists all available commands. 'github-runner-cli' is the full executable name.\n")
	return nil
}

func printHelpSection(title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("\n%s:\n", title)
	for _, v := range values {
		fmt.Printf("  %s\n", v)
	}
}

const globalHelp = `github-runner-cli (ghr) — GitHub Actions runner manager

Manages repository-specific GitHub Actions runners using Docker/Colima.
Use 'ghr help <command>' or 'ghr <command> --help' for full documentation,
parameters, valid values and examples. Help never requires a runner or Docker.

USAGE:
  ghr help [command [subcommand]]
  ghr <command> --help
  ghr <command> [options]

RUNNER COMMANDS:
  requirements        Analyze workflow actions and toolchain requirements
  build               Prepare and verify a Docker runner image
  doctor              Verify image toolchain against workflow requirements
  install             Build, register and create a repository runner container
  start               Start a registered runner
  stop                Stop a runner (check busy state before stopping)
  status              Inspect container status and GitHub online/busy state
  monitor             Watch status, recent runs and logs (Ctrl+C to quit)
  refresh             Replace a stopped container, preserving its volume
  list-installed      List local runners; --repo also lists GitHub registrations
  uninstall           Safely unregister/remove one or all local runners
  self-uninstall      Remove only the installed CLI and runtime assets
  runner              Advanced Docker commands and workflow setup
  version             Print the CLI version
  help                Show this overview or detailed command help

COMMON OPTIONS:
  --repo OWNER/REPO         GitHub repository (alias: --repository, or =VALUE)
  --allow-public            Explicit consent for public repository registration
  --dry-run                 Preview uninstallation without modifying anything
  --delete-data             Delete runner volume; irreversible
  --all                     Select all local installations for uninstall/listing
  --yes                     Required for bulk removal and self-uninstall
  -h, --help                Show help at any command level

GLOBAL ENVIRONMENT:
  RUNNER_BACKEND            auto (default), docker
  RUNNER_PROFILE            auto (default), generic, toolchain, runner-base
  RUNNER_IMAGE              Optional explicit Docker image tag
  RUNNER_DOCKER_SOCKET      0 (default), 1 (privileged opt-in)
  RUNNER_MONITOR_SECONDS    Poll interval in seconds: integer >= 2; default 10
  GITHUB_RUNNER_CLI_ENV     Override installed runtime assets directory

GETTING STARTED:
  ghr requirements --repo example-org/demo-app --output toolchain.yaml
  ghr build --repo example-org/demo-app
  ghr doctor --repo example-org/demo-app
  ghr install --repo example-org/demo-app --allow-public
  ghr start --repo example-org/demo-app
  ghr status --repo example-org/demo-app
  ghr monitor --repo example-org/demo-app

MORE HELP:
  ghr help status
  ghr status --help
  ghr help uninstall
  ghr runner --help
  ghr help runner setup
  ghr runner setup --help

The full binary name is 'github-runner-cli'; 'ghr' is a shell shortcut if
configured. Help works without GitHub credentials, Docker or a configured repo.
`
