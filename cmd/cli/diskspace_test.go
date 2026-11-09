package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiskspaceOptions(t *testing.T) {
	for _, tc := range []struct {
		args []string
		mode string
	}{
		{nil, "list"},
		{[]string{"--list"}, "list"},
		{[]string{"--monitor", "--interval", "2s"}, "monitor"},
		{[]string{"--clean"}, "clean"},
		{[]string{"--clean", "--yes"}, "clean"},
	} {
		got, err := parseDiskspaceArgs(tc.args)
		if err != nil || got.mode != tc.mode {
			t.Errorf("%q: %+v, %v", tc.args, got, err)
		}
	}
	for _, args := range [][]string{
		{"--list", "--clean"},
		{"--yes"},
		{"--monitor", "--yes"},
		{"--interval", "5s"},
		{"--monitor", "--interval", "invalid"},
		{"--monitor", "--interval", "0s"},
		{"--clean", "--interval", "2s"},
		{"--doctor", "--yes"},
		{"--doctor", "--monitor"},
		{"--unknown"},
	} {
		if _, err := parseDiskspaceArgs(args); err == nil {
			t.Errorf("accepted invalid arguments: %v", args)
		}
	}
}

func TestParseColimaDFAndHealth(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{"Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vdb1 100000000 80000000 20000000 80% /mnt/lima-colima\n", "OK"},
		{"Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vdb1 100000000 90000000 10000000 90% /mnt/lima-colima\n", "WARNING"},
		{"Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vdb1 100000000 99000000 1000000 99% /mnt/lima-colima\n", "CRITICAL"},
	} {
		got, err := parseColimaDF(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		if health := diskHealth(got); health != tc.want {
			t.Errorf("got %s want %s", health, tc.want)
		}
	}
	if _, err := parseColimaDF("bad"); err == nil {
		t.Fatal("accepted malformed filesystem output")
	}
}

func TestDiskspaceCleanPreviewNeverTouchesDocker(t *testing.T) {
	count := 0
	runner := diskspaceRunner{run: func(context.Context, string, ...string) (string, error) {
		count++
		return "", errors.New("unexpected external invocation")
	}}
	var out bytes.Buffer
	if err := diskspaceCommand(context.Background(), []string{"--clean"}, runner, &out); err != nil {
		t.Fatal(err)
	}
	if count != 0 || !strings.Contains(out.String(), "DRY RUN") {
		t.Fatalf("preview invoked %d commands: %s", count, out.String())
	}
}

func TestDiskspaceCleanOnlyPrunesCacheAndDanglingImages(t *testing.T) {
	var calls []string
	runner := diskspaceRunner{run: func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "colima" {
			return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vdb1 100000000 70000000 30000000 70% /mnt/lima-colima\n", nil
		}
		return "ok\n", nil
	}}
	var out bytes.Buffer
	if err := diskspaceCommand(context.Background(), []string{"--clean", "--yes"}, runner, &out); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker builder prune --all --force",
		"docker image prune --force",
		"colima ssh -- df -Pk /mnt/lima-colima",
		"docker system df",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls: %v", calls)
	}
	for i, call := range calls {
		if call != want[i] {
			t.Errorf("call %d: %q want %q", i, call, want[i])
		}
	}
}

func TestDiskspaceDoctorParsers(t *testing.T) {
	o, err := parseDiskspaceArgs([]string{"--doctor"})
	if err != nil || o.mode != "doctor" { t.Fatalf("doctor options: %+v %v", o, err) }
	rows, err := readDockerDiskRows("{\"Type\":\"Build Cache\",\"Size\":\"9.8GB\",\"Reclaimable\":\"7.2GB\"}\n")
	if err != nil || len(rows) != 1 || parseDockerBytes(rows[0].Reclaimable) < 7e9 {
		t.Fatalf("invalid Docker cache parsing: %+v %v", rows, err)
	}
	sizes := parseDU("1024\t/home/small\n4096\t/home/large\n")
	if len(sizes) != 2 || sizes[0].Path != "/home/large" {
		t.Fatalf("unexpected du sizes: %+v", sizes)
	}
}

func TestDiskspaceDoctorNeverPrunes(t *testing.T) {
	var calls []string
	runner := diskspaceRunner{run: func(_ context.Context, name string, args ...string) (string, error) {
		command := name+" "+strings.Join(args, " ")
		calls = append(calls, command)
		if strings.Contains(command, "prune") || strings.Contains(command, "rm ") {
			return "", errors.New("doctor attempted destructive operation")
		}
		switch name {
		case "colima", "df":
			return "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vdb1 100000000 98000000 2000000 98% /mnt/lima-colima\n", nil
		case "docker":
			return "{\"Type\":\"Build Cache\",\"Size\":\"9.8GB\",\"Reclaimable\":\"7.2GB\",\"Active\":0}\n", nil
		case "du":
			return "1024\t/safe\n", nil
		}
		return "", errors.New("not installed")
	}}
	var out bytes.Buffer
	if err := diskspaceCommand(context.Background(), []string{"--doctor"}, runner, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "CRITICAL") || !strings.Contains(out.String(), "Recommended actions") ||
		!strings.Contains(out.String(), "builder prune") {
		t.Fatalf("missing useful recommendations: %s", out.String())
	}
	for _, call := range calls {
		if strings.Contains(call, "prune") {
			t.Fatalf("doctor ran a destructive command: %s", call)
		}
	}
}
