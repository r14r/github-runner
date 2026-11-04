// Package monitor exposes Docker-local runner logs and diagnostic events.
// It never calls GitHub and does not modify runner state.
package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Type    string    `json:"type"`
	Runner  string    `json:"runner"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}
type Runner struct {
	Repository string `json:"repository"`
	Container  string `json:"container"`
	Status     string `json:"status"`
}
type Service struct {
	runners map[string]Runner
}

func New(runners []Runner) *Service {
	s := &Service{runners: make(map[string]Runner)}
	for _, r := range runners {
		s.runners[r.Container] = r
	}
	return s
}
func (s *Service) Runners() []Runner {
	result := make([]Runner, 0, len(s.runners))
	for _, r := range s.runners {
		out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", r.Container).Output()
		if err != nil {
			r.Status = "unavailable"
		} else {
			r.Status = strings.TrimSpace(string(out))
		}
		result = append(result, r)
	}
	return result
}
func (s *Service) Stream(ctx context.Context, container string, tail int, emit func(Event)) error {
	if _, ok := s.runners[container]; !ok {
		return fmt.Errorf("unknown local runner %q", container)
	}
	if tail < 0 || tail > 1000 {
		return fmt.Errorf("invalid tail %d", tail)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	send := func(source, msg string) {
		mu.Lock()
		defer mu.Unlock()
		emit(Event{Type: "log", Runner: container, Source: source, Message: msg, Time: time.Now().UTC()})
	}
	follow := func(source string, cmd *exec.Cmd) error {
		stdout, e := cmd.StdoutPipe()
		if e != nil {
			return e
		}
		stderr, e := cmd.StderrPipe()
		if e != nil {
			return e
		}
		if e = cmd.Start(); e != nil {
			return e
		}
		wg.Add(2)
		scan := func(reader io.Reader) {
			defer wg.Done()
			sc := bufio.NewScanner(reader)
			sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
			for sc.Scan() {
				send(source, sc.Text())
			}
		}
		go scan(stdout)
		go scan(stderr)
		e = cmd.Wait()
		return e
	}
	// Diagnostic files are a runner implementation detail: observe, do not mutate.
	// This shell watches new Worker_*/Runner_* files without depending on the GitHub API.
	var errs []error
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, "docker", "exec", container, "sh", "-c",
			"cd /runner/_diag 2>/dev/null || exit 0; tail -n 0 -F Runner_*.log Worker_*.log 2>/dev/null")
		if e := follow("diag", cmd); e != nil && ctx.Err() == nil {
			mu.Lock()
			errs = append(errs, e)
			mu.Unlock()
		}
	}()
	err := follow("docker", exec.CommandContext(ctx, "docker", "logs", "--follow", "--tail", strconv.Itoa(tail), container))
	cancel()
	wg.Wait()
	if err != nil {
		return err
	}
	_ = errs
	return nil
}
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/monitor", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"runners": s.Runners()})
	})
	mux.HandleFunc("/api/monitor/runners", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Runners())
	})
	mux.HandleFunc("/api/monitor/runners/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/monitor/runners/")
		parts := strings.Split(path, "/")
		id := parts[0]
		if _, ok := s.runners[id]; !ok {
			http.Error(w, "runner not found", 404)
			return
		}
		if len(parts) == 1 {
			for _, runner := range s.Runners() {
				if runner.Container == id {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(runner)
					return
				}
			}
		}
		if len(parts) != 2 || (parts[1] != "events" && parts[1] != "logs") {
			http.NotFound(w, r)
			return
		}
		tail := 10
		if v := r.URL.Query().Get("tail"); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 0 || n > 1000 {
				http.Error(w, "invalid tail", 400)
				return
			}
			tail = n
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if parts[1] == "events" {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		flush, _ := w.(http.Flusher)
		_ = s.Stream(r.Context(), id, tail, func(e Event) {
			if parts[1] == "events" {
				b, _ := json.Marshal(e)
				fmt.Fprintf(w, "event: %s\\ndata: %s\\n\\n", e.Type, b)
			} else {
				fmt.Fprintf(w, "[%s] %s\\n", e.Source, e.Message)
			}
			if flush != nil {
				flush.Flush()
			}
		})
	})
	return mux
}
