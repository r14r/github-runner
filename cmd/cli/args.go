package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// repoOption accepts --repo and --repository in any position, including
// --repo=OWNER/REPO, and leaves only action-specific flags for the caller.
// An explicit flag takes precedence over the optional RUNNER_REPO fallback.
func repoOption(args []string) (string, []string, error) {
	var repo string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var value string
		switch {
		case arg == "--repo" || arg == "--repository":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", nil, fmt.Errorf("%s requires OWNER/REPO", arg)
			}
			i++
			value = args[i]
		case strings.HasPrefix(arg, "--repo="):
			value = strings.TrimPrefix(arg, "--repo=")
		case strings.HasPrefix(arg, "--repository="):
			value = strings.TrimPrefix(arg, "--repository=")
		default:
			rest = append(rest, arg)
			continue
		}
		if repo != "" {
			return "", nil, errors.New("repository specified multiple times")
		}
		value = gitHubRepository(value)
		if _, err := repoSlug(value); err != nil {
			return "", nil, err
		}
		repo = value
	}
	if repo == "" {
		if fallback := strings.TrimSpace(os.Getenv("RUNNER_REPO")); fallback != "" {
			fallback = gitHubRepository(fallback)
			if _, err := repoSlug(fallback); err != nil {
				return "", nil, fmt.Errorf("invalid RUNNER_REPO: %w", err)
			}
			repo = fallback
		}
	}
	return repo, rest, nil
}

func requireRepo(args []string) (string, []string, error) {
	repo, rest, err := repoOption(args)
	if err != nil {
		return "", nil, err
	}
	if repo == "" {
		return "", nil, errors.New("missing repository: use --repo OWNER/REPO or set RUNNER_REPO")
	}
	return repo, rest, nil
}

func noExtras(action string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("%s: unexpected arguments %q", action, args)
	}
	return nil
}
