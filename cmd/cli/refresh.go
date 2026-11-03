package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// refreshRuntime isolates Docker and GitHub operations so refresh lifecycle
// safety is tested without modifying actual runner registrations or containers.
type refreshRuntime struct {
	exists  func(kind, name string) bool
	inspect func(format string) (string, error)
	local   func() (localRunner, error)
	remote  func() ([]runnerRecord, error)
	docker  func(args ...string) error
	create  func() error
}

// checkRefreshIdle fails closed: never interrupt a runner if GitHub status or
// the persisted runner identity cannot be verified. No registration is deleted.
func (r dockerRunner) checkRefreshIdle(rt refreshRuntime) error {
	l, err := rt.local()
	if err != nil {
		return fmt.Errorf("cannot read registered runner identity; refusing to stop: %w", err)
	}
	if l.Repository != "" && !strings.EqualFold(l.Repository, r.Repo) {
		return fmt.Errorf("container belongs to %s, not %s; refusing to stop", l.Repository, r.Repo)
	}
	if l.Identity.AgentID == 0 || l.Identity.AgentName == "" {
		return errors.New("cannot verify runner ID and name; refusing to stop a running container")
	}
	remote, err := rt.remote()
	if err != nil {
		return fmt.Errorf("GitHub runner status unavailable; refusing to stop: %w", err)
	}
	registration, err := matchedRegistration(l.Identity, remote)
	if err != nil {
		return fmt.Errorf("GitHub runner identity mismatch; refusing to stop: %w", err)
	}
	if registration == nil {
		return fmt.Errorf("runner %s (%d) is not registered on GitHub; refusing to stop", l.Identity.AgentName, l.Identity.AgentID)
	}
	if registration.Busy {
		return fmt.Errorf("runner %s is busy with a GitHub Actions job; wait for job completion before refresh", registration.Name)
	}
	fmt.Printf("    GitHub runner: %s (ID %d, idle)\n", registration.Name, registration.ID)
	return nil
}

// refreshWithRuntime preserves the previous run state. A running, idle runner
// is stopped, recreated from the desired image, and restarted. A previously
// stopped runner remains stopped. The registration volume is never removed.
func (r dockerRunner) refreshWithRuntime(rt refreshRuntime) error {
	fmt.Printf("Refreshing runner: %s\n", r.Repo)
	exists := rt.exists("container", r.Container)
	running := false
	if exists {
		value, err := rt.inspect("{{.State.Running}}")
		if err != nil {
			return fmt.Errorf("cannot inspect %s: %w", r.Container, err)
		}
		if value != "true" && value != "false" {
			return fmt.Errorf("unknown container running state %q; refusing refresh", value)
		}
		running = value == "true"

		// Preserve all existing privileged Docker mounts, or fail BEFORE stop.
		mounts, err := rt.inspect(`{{range .Mounts}}{{if eq .Destination "/var/run/docker.sock"}}yes{{end}}{{end}}`)
		if err != nil {
			return err
		}
		if mounts == "yes" && os.Getenv("RUNNER_DOCKER_SOCKET") != "1" {
			return errors.New("runner uses a privileged Docker socket mount; set RUNNER_DOCKER_SOCKET=1 explicitly to preserve it (nothing stopped)")
		}
		if mounts != "" && mounts != "yes" {
			return fmt.Errorf("unexpected Docker socket mount inspection %q; refusing refresh", mounts)
		}
	}

	if running {
		// Check the exact persisted GitHub identity and busy flag immediately
		// before interrupting the running container. A remote API failure must
		// not silently be interpreted as an idle runner.
		if err := r.checkRefreshIdle(rt); err != nil {
			return err
		}
		fmt.Printf("01: Stop idle container %s\n", r.Container)
		if err := rt.docker("stop", r.Container); err != nil {
			return fmt.Errorf("cannot stop %s; refresh aborted: %w", r.Container, err)
		}
	}
	if exists {
		fmt.Printf("02: Replace container %s\n", r.Container)
		if err := rt.docker("rm", r.Container); err != nil {
			// If replacement fails before the container is removed, try to
			// restore the previous running state without touching the volume.
			if running {
				if restartErr := rt.docker("start", r.Container); restartErr != nil {
					return fmt.Errorf("remove failed: %v; recovery restart also failed: %w", err, restartErr)
				}
			}
			return fmt.Errorf("cannot remove old container %s: %w", r.Container, err)
		}
	} else {
		fmt.Printf("02: Create container %s\n", r.Container)
	}
	if err := rt.create(); err != nil {
		return fmt.Errorf("cannot create replacement container %s: %w (registration volume %s preserved; runner currently stopped)", r.Container, err, r.Volume)
	}
	if running {
		fmt.Printf("03: Restart container %s\n", r.Container)
		if err := rt.docker("start", r.Container); err != nil {
			return fmt.Errorf("new container created but could not restart %s: %w (volume preserved; run ghr start --repo %s)", r.Container, err, r.Repo)
		}
	} else {
		fmt.Println("03: Container remains stopped (previous state preserved)")
	}
	fmt.Printf("RESULT: OK — image %s; registration volume %s retained\n", r.Image, r.Volume)
	return nil
}
