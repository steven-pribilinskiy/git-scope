// Package profiling collects timing data for git-scope's hot paths and exposes
// it as an on-exit summary plus an optional pprof HTTP server. All collection
// is gated behind the `Enabled` flag — when off, every helper is a near-zero
// no-op (one atomic-bool check) so production paths pay nothing.
package profiling

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	enabled atomic.Bool

	mu        sync.Mutex
	phases    map[string]*phaseStat
	repoTimes []repoTiming
	startedAt time.Time
	logPath   string
)

type phaseStat struct {
	calls   int
	total   time.Duration
	maxOne  time.Duration
}

type repoTiming struct {
	Path    string
	Status  time.Duration
	GitLog  time.Duration
}

// Enabled reports whether profiling collection is active.
func Enabled() bool { return enabled.Load() }

// Enable turns on collection. `logFile` is the path that WriteSummary will
// write to on shutdown; if empty, the summary is written to stderr.
// `pprofAddr` (e.g. "localhost:6060") starts the pprof HTTP server in the
// background — empty disables it.
func Enable(logFile, pprofAddr string) error {
	mu.Lock()
	phases = make(map[string]*phaseStat)
	repoTimes = nil
	startedAt = time.Now()
	logPath = logFile
	mu.Unlock()
	enabled.Store(true)

	if pprofAddr != "" {
		go func() {
			_ = http.ListenAndServe(pprofAddr, nil)
		}()
	}

	if logFile != "" {
		if dir := filepath.Dir(logFile); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}
	return nil
}

// LogPath returns the configured summary destination ("" → stderr).
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Phase starts a named-phase timer. The returned func must be called to record
// the elapsed time. When disabled the returned func is still safe to call.
func Phase(name string) func() {
	if !enabled.Load() {
		return func() {}
	}
	start := time.Now()
	return func() {
		d := time.Since(start)
		mu.Lock()
		defer mu.Unlock()
		s, ok := phases[name]
		if !ok {
			s = &phaseStat{}
			phases[name] = s
		}
		s.calls++
		s.total += d
		if d > s.maxOne {
			s.maxOne = d
		}
	}
}

// RecordRepo logs per-repo git timings (Status + Log) so the summary can show
// the slowest repos. Cheap; can be called from a worker goroutine.
func RecordRepo(path string, statusDur, logDur time.Duration) {
	if !enabled.Load() {
		return
	}
	mu.Lock()
	repoTimes = append(repoTimes, repoTiming{Path: path, Status: statusDur, GitLog: logDur})
	mu.Unlock()
}

// WriteSummary dumps a human-readable summary to the configured destination.
// Safe to call multiple times. No-op when profiling never ran.
func WriteSummary() {
	if !enabled.Load() {
		return
	}
	dst := io.Writer(os.Stderr)
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			defer f.Close()
			dst = f
		}
	}
	mu.Lock()
	defer mu.Unlock()

	total := time.Since(startedAt)
	fmt.Fprintf(dst, "git-scope profile summary\n")
	fmt.Fprintf(dst, "  total wall:     %s\n", total)
	fmt.Fprintf(dst, "  GOMAXPROCS:     %d\n", runtime.GOMAXPROCS(0))
	fmt.Fprintf(dst, "  NumCPU:         %d\n", runtime.NumCPU())
	fmt.Fprintf(dst, "  repos timed:    %d\n", len(repoTimes))
	fmt.Fprintln(dst)

	type kv struct {
		name string
		s    *phaseStat
	}
	rows := make([]kv, 0, len(phases))
	for n, s := range phases {
		rows = append(rows, kv{n, s})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].s.total > rows[j].s.total })

	fmt.Fprintf(dst, "phases (sorted by total wall):\n")
	fmt.Fprintf(dst, "  %-40s %8s %12s %12s %12s\n", "name", "calls", "total", "avg", "max")
	for _, r := range rows {
		avg := time.Duration(0)
		if r.s.calls > 0 {
			avg = r.s.total / time.Duration(r.s.calls)
		}
		fmt.Fprintf(dst, "  %-40s %8d %12s %12s %12s\n", r.name, r.s.calls, r.s.total, avg, r.s.maxOne)
	}
	fmt.Fprintln(dst)

	rt := append([]repoTiming(nil), repoTimes...)
	sort.Slice(rt, func(i, j int) bool {
		return (rt[i].Status + rt[i].GitLog) > (rt[j].Status + rt[j].GitLog)
	})
	top := 20
	if len(rt) < top {
		top = len(rt)
	}
	fmt.Fprintf(dst, "slowest %d repos (status + last-commit):\n", top)
	fmt.Fprintf(dst, "  %12s %12s %12s  %s\n", "total", "status", "log", "path")
	for i := 0; i < top; i++ {
		t := rt[i]
		fmt.Fprintf(dst, "  %12s %12s %12s  %s\n", t.Status+t.GitLog, t.Status, t.GitLog, t.Path)
	}
}
