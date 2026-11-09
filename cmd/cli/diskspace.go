package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

type diskspaceOptions struct {
	mode     string
	yes      bool
	interval time.Duration
}

func parseDiskspaceArgs(args []string) (diskspaceOptions, error) {
	o := diskspaceOptions{mode: "list", interval: 30 * time.Second}
	modeSeen, intervalSeen := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--list", "--monitor", "--clean", "--doctor":
			if modeSeen {
				return o, errors.New("specify only one of --list, --monitor, --clean or --doctor")
			}
			o.mode, modeSeen = strings.TrimPrefix(arg, "--"), true
		case "--yes":
			if o.yes {
				return o, errors.New("--yes specified more than once")
			}
			o.yes = true
		case "--interval":
			if i+1 == len(args) {
				return o, errors.New("--interval requires duration, e.g. 30s")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil || d < time.Second {
				return o, errors.New("--interval must be a duration >= 1s")
			}
			o.interval = d
			intervalSeen = true
		default:
			return o, fmt.Errorf("unknown diskspace option %q", arg)
		}
	}
	if o.yes && o.mode != "clean" {
		return o, errors.New("--yes is only valid with --clean")
	}
	if o.mode != "monitor" && intervalSeen {
		return o, errors.New("--interval is only valid with --monitor")
	}
	return o, nil
}

type diskspaceRunner struct {
	run func(ctx context.Context, name string, args ...string) (string, error)
}

func systemDiskspaceRunner() diskspaceRunner {
	return diskspaceRunner{run: func(ctx context.Context, name string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}}
}

type diskSnapshot struct {
	totalKB int64
	usedKB  int64
	availKB int64
	mount   string
}

func parseColimaDF(output string) (diskSnapshot, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return diskSnapshot{}, errors.New("unexpected df output")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 6 {
		return diskSnapshot{}, errors.New("unexpected df columns")
	}
	total, e1 := strconv.ParseInt(fields[1], 10, 64)
	used, e2 := strconv.ParseInt(fields[2], 10, 64)
	avail, e3 := strconv.ParseInt(fields[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || total <= 0 {
		return diskSnapshot{}, errors.New("invalid df capacity")
	}
	return diskSnapshot{totalKB: total, usedKB: used, availKB: avail, mount: fields[5]}, nil
}

func diskHealth(s diskSnapshot) string {
	freeGB := float64(s.availKB) / (1024 * 1024)
	if freeGB < 5 || float64(s.usedKB)/float64(s.totalKB) >= 0.95 {
		return "CRITICAL"
	}
	if freeGB < 15 || float64(s.usedKB)/float64(s.totalKB) >= 0.85 {
		return "WARNING"
	}
	return "OK"
}

func diskspaceSnapshot(ctx context.Context, runner diskspaceRunner, out io.Writer) error {
	// Docker storage is hosted on the Colima data filesystem, not the guest root filesystem.
	df, err := runner.run(ctx, "colima", "ssh", "--", "df", "-Pk", "/mnt/lima-colima")
	if err != nil {
		fmt.Fprintf(out, "Colima storage: unavailable (%v)\n", err)
	} else {
		info, parseErr := parseColimaDF(df)
		if parseErr != nil {
			fmt.Fprintf(out, "Colima storage: unavailable (%v)\n", parseErr)
		} else {
			fmt.Fprintf(out, "Colima: %s; total=%.1f GiB used=%.1f GiB free=%.1f GiB [%s]\n",
				info.mount, float64(info.totalKB)/(1024*1024), float64(info.usedKB)/(1024*1024),
				float64(info.availKB)/(1024*1024), diskHealth(info))
			if diskHealth(info) != "OK" {
				fmt.Fprintln(out, "WARNING: Low Docker disk capacity may prevent runner startup or jobs.")
			}
		}
	}
	data, err := runner.run(ctx, "docker", "system", "df")
	if err != nil {
		return fmt.Errorf("Docker storage unavailable: %w", err)
	}
	fmt.Fprintln(out, "Docker disk usage:")
	fmt.Fprint(out, data)
	return nil
}

func diskspaceClean(ctx context.Context, runner diskspaceRunner, out io.Writer, yes bool) error {
	fmt.Fprintln(out, "Cleanup scope: unused build cache and dangling images only.")
	fmt.Fprintln(out, "Runner volumes, containers, active images and registrations are never deleted.")
	if !yes {
		fmt.Fprintln(out, "DRY RUN: proposed commands:")
		fmt.Fprintln(out, "  docker builder prune --all --force")
		fmt.Fprintln(out, "  docker image prune --force")
		fmt.Fprintln(out, "Run ghr diskspace --clean --yes to execute.")
		return nil
	}
	for _, args := range [][]string{
		{"builder", "prune", "--all", "--force"},
		{"image", "prune", "--force"},
	} {
		data, err := runner.run(ctx, "docker", args...)
		if err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
		fmt.Fprint(out, data)
	}
	return diskspaceSnapshot(ctx, runner, out)
}

func diskspaceCommand(ctx context.Context, args []string, runner diskspaceRunner, out io.Writer) error {
	o, err := parseDiskspaceArgs(args)
	if err != nil {
		return err
	}
	switch o.mode {
	case "doctor":
		return diskspaceDoctor(ctx, runner, out)
	case "list":
		return diskspaceSnapshot(ctx, runner, out)
	case "clean":
		return diskspaceClean(ctx, runner, out, o.yes)
	case "monitor":
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
		defer stop()
		ticker := time.NewTicker(o.interval)
		defer ticker.Stop()
		for {
			fmt.Fprintf(out, "\nDiskspace check: %s\n", time.Now().Format(time.RFC3339))
			if err := diskspaceSnapshot(ctx, runner, out); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	}
	return errors.New("unsupported diskspace mode")
}
