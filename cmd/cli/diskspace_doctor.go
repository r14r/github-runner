package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type dockerDiskRow struct {
	Type        string      `json:"Type"`
	TotalCount  int64       `json:"TotalCount"`
	Active      int64       `json:"Active"`
	Size        interface{} `json:"Size"`
	Reclaimable interface{} `json:"Reclaimable"`
}

func parseDockerBytes(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		n = strings.TrimSpace(strings.Split(n, " (")[0])
		fields := strings.Fields(n)
		if len(fields) == 0 { return 0 }
		val := fields[0]
		for i, c := range val {
			if (c < '0' || c > '9') && c != '.' {
				amount, err := strconv.ParseFloat(val[:i], 64)
				if err != nil { return 0 }
				switch strings.ToLower(val[i:]) {
				case "b": return int64(amount)
				case "kb": return int64(amount * 1000)
				case "mb": return int64(amount * 1e6)
				case "gb": return int64(amount * 1e9)
				case "tb": return int64(amount * 1e12)
				case "kib": return int64(amount * 1024)
				case "mib": return int64(amount * 1024 * 1024)
				case "gib": return int64(amount * 1024 * 1024 * 1024)
				}
				return 0
			}
		}
		if value, err := strconv.ParseFloat(val, 64); err == nil { return int64(value) }
	}
	return 0
}

func readDockerDiskRows(raw string) ([]dockerDiskRow, error) {
	rows := []dockerDiskRow{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" { continue }
		var row dockerDiskRow
		if err := json.Unmarshal([]byte(line), &row); err != nil { return nil, err }
		rows = append(rows, row)
	}
	if len(rows) == 0 { return nil, fmt.Errorf("empty docker system df response") }
	return rows, nil
}

type directorySize struct {
	Path string
	Bytes int64
}

func parseDU(raw string) []directorySize {
	var entries []directorySize
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 { continue }
		kb, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || kb < 0 { continue }
		entries = append(entries, directorySize{Path: parts[1], Bytes: kb * 1024})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Bytes > entries[j].Bytes })
	if len(entries) > 10 { entries = entries[:10] }
	return entries
}

func bytesGiB(v int64) float64 { return float64(v) / (1024 * 1024 * 1024) }

// --doctor only uses read-only external commands; cleanup is intentionally
// available only through a separately confirmed --clean --yes invocation.
func diskspaceDoctor(ctx context.Context, runner diskspaceRunner, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	fmt.Fprintln(out, "GitHub Runner — Diskspace Doctor")
	var recommendations []string
	var problems []string
	report := func(name string, args ...string) (string, bool) {
		raw, err := runner.run(ctx, name, args...)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			fmt.Fprintf(out, "  UNAVAILABLE: %s (%v)\n", name, err)
			return "", false
		}
		return raw, true
	}

	fmt.Fprintln(out, "\n01: Colima data filesystem")
	if raw, ok := report("colima", "ssh", "--", "df", "-Pk", "/mnt/lima-colima"); ok {
		if disk, err := parseColimaDF(raw); err != nil {
			fmt.Fprintf(out, "  UNAVAILABLE: %v\n", err)
			problems = append(problems, "Colima df parse error")
		} else {
			health := diskHealth(disk)
			fmt.Fprintf(out, "  %s: %s — %.1f GiB used, %.1f GiB free (%.1f GiB total)\n",
				health, disk.mount, float64(disk.usedKB)/(1024*1024), float64(disk.availKB)/(1024*1024), float64(disk.totalKB)/(1024*1024))
			if health != "OK" { recommendations = append(recommendations, "Colima storage is "+health+": prune unused build cache and dangling images; consider a larger Colima disk after checking host free space.") }
		}
	}
	fmt.Fprintln(out, "\n02: Host filesystem")
	home, err := os.UserHomeDir()
	if err != nil { home = "." }
	home = filepath.Clean(home)
	if raw, ok := report("df", "-Pk", home); ok {
		if disk, err := parseColimaDF(raw); err == nil {
			fmt.Fprintf(out, "  %s: %s — %.1f GiB used, %.1f GiB free (%.1f GiB total)\n",
				diskHealth(disk), disk.mount, float64(disk.usedKB)/(1024*1024), float64(disk.availKB)/(1024*1024), float64(disk.totalKB)/(1024*1024))
			if diskHealth(disk) != "OK" { recommendations = append(recommendations, "Host filesystem has low free space: review large directories before expanding Colima.") }
		} else { problems = append(problems, "Host df parse error") }
	}

	fmt.Fprintln(out, "\n03: Docker disk usage and reclaimable cache")
	if raw, ok := report("docker", "system", "df", "--format", "{{json .}}"); ok {
		rows, err := readDockerDiskRows(raw)
		if err != nil {
			fmt.Fprintf(out, "  Cannot interpret Docker data: %v\n", err)
			problems = append(problems, "Docker usage parsing failed")
		} else {
			for _, row := range rows {
				fmt.Fprintf(out, "  %-15s size=%v reclaimable=%v active=%d\n", row.Type, row.Size, row.Reclaimable, row.Active)
				if strings.Contains(strings.ToLower(row.Type), "build cache") && parseDockerBytes(row.Reclaimable) > 1024*1024*1024 {
					recommendations = append(recommendations, "Unused build cache can be reclaimed: docker builder prune --all (review first).")
				}
				if strings.Contains(strings.ToLower(row.Type), "images") && parseDockerBytes(row.Reclaimable) > 1024*1024*1024 {
					recommendations = append(recommendations, "Unused images may be removable: docker image prune (dangling only), or inspect docker image ls before broader pruning.")
				}
			}
		}
	}
	fmt.Fprintln(out, "\n04: Top 10 host directories (direct children of HOME)")
	// Avoid traversing the entire filesystem or crossing mounted volumes; only
	// immediate home children, sorted by size. 'du' may lack permission for
	// some macOS-managed folders, in which case it reports a partial result.
	roots := []string{home}
	cloud := "/Users/Shared/CLOUD"
	if cloud != home {
		if st, err := os.Stat(cloud); err == nil && st.IsDir() { roots = append(roots, cloud) }
	}
	var paths []string
	for _, root := range roots {
		items, err := os.ReadDir(root)
		if err != nil {
			problems = append(problems, "Cannot list "+root+": "+err.Error())
			continue
		}
		for _, item := range items {
			if item.IsDir() { paths = append(paths, filepath.Join(root, item.Name())) }
		}
	}
	var measured []directorySize
	for _, path := range paths {
		// Restrict individual scans so an enormous or inaccessible tree
		// does not block the entire doctor report.
		scanCtx, stop := context.WithTimeout(ctx, 8*time.Second)
		raw, err := runner.run(scanCtx, "du", "-sk", path)
		stop()
		if err != nil {
			fmt.Fprintf(out, "  SKIPPED: %s (%v)\n", path, err)
			continue
		}
		measured = append(measured, parseDU(raw)...)
	}
	sort.Slice(measured, func(i, j int) bool { return measured[i].Bytes > measured[j].Bytes })
	if len(measured) > 10 { measured = measured[:10] }
	for i, entry := range measured { fmt.Fprintf(out, "  %2d. %7.2f GiB  %s\n", i+1, bytesGiB(entry.Bytes), entry.Path) }
	if len(measured) == 0 { problems = append(problems, "No host directories measured") }

	fmt.Fprintln(out, "\nRecommended actions:")
	for i, rec := range recommendations { fmt.Fprintf(out, "  %d. %s\n", i+1, rec) }
	if len(recommendations) == 0 { fmt.Fprintln(out, "  No urgent cleanup indicated by available metrics.") }
	fmt.Fprintln(out, "  Preview the safe cleanup with: ghr diskspace --clean")
	fmt.Fprintln(out, "  Execute only after reviewing: ghr diskspace --clean --yes")
	if len(problems) != 0 { fmt.Fprintf(out, "  NOTE: %d diagnostic(s) unavailable; findings may be incomplete.\n", len(problems)) }
	return nil
}
