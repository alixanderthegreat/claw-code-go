// metrics.go exposes a Prometheus text-exposition-format /metrics endpoint over the
// Registry's own dispatch state - "visibility first" (the operator's own call,
// 2026-09-22): before claw-code-go dispatches anything unattended, there should
// already be a place to watch it, the same way kronk's own /metrics (wired into
// this stack the same day) is what actually caught the system-prompt caching bug
// earlier in this session.
//
// Hand-rolled rather than github.com/prometheus/client_golang: the metric set here
// is small and fixed, and this keeps internal/serve free of new dependencies,
// matching the rest of the package (no new deps for the dispatch surface itself
// either). Computed fresh from Registry.m on every scrape - the same "read live
// state, don't cache" approach Status() already uses - rather than counters
// incremented at transition time, so a scrape always reflects exactly what the
// registry currently holds.
package serve

import (
	"fmt"
	"strings"
)

// dispatchCounts is the pure aggregation behind Metrics - kept free of the
// Registry/mutex/*exec.Cmd machinery so it's directly unit-testable against a
// plain slice of states, the same spirit as classifyStatus.
type dispatchCounts struct {
	running, done, failed, interrupted int
	durationSum                        float64 // seconds, finished dispatches only
	finishedCount                      int
}

func aggregateCounts(states []dispatchStatus, durations []float64) dispatchCounts {
	var c dispatchCounts
	for i, s := range states {
		switch s {
		case statusRunning:
			c.running++
		case statusDone:
			c.done++
		case statusFailed:
			c.failed++
		case statusInterrupted:
			c.interrupted++
		}
		if s != statusRunning {
			c.durationSum += durations[i]
			c.finishedCount++
		}
	}
	return c
}

// render writes c in Prometheus text exposition format.
func (c dispatchCounts) render() string {
	var b strings.Builder
	b.WriteString("# HELP claw_dispatches_active Dispatches currently running right now.\n")
	b.WriteString("# TYPE claw_dispatches_active gauge\n")
	fmt.Fprintf(&b, "claw_dispatches_active %d\n\n", c.running)

	b.WriteString("# HELP claw_dispatch_total Dispatches this server process has seen, by terminal status. In-memory only - resets on restart, same limitation as the registry itself (see internal/serve's own package doc).\n")
	b.WriteString("# TYPE claw_dispatch_total counter\n")
	fmt.Fprintf(&b, "claw_dispatch_total{status=\"done\"} %d\n", c.done)
	fmt.Fprintf(&b, "claw_dispatch_total{status=\"failed\"} %d\n", c.failed)
	fmt.Fprintf(&b, "claw_dispatch_total{status=\"interrupted\"} %d\n\n", c.interrupted)

	b.WriteString("# HELP claw_dispatch_duration_seconds_sum Sum of wall-clock duration for finished dispatches (done/failed/interrupted).\n")
	b.WriteString("# TYPE claw_dispatch_duration_seconds_sum counter\n")
	fmt.Fprintf(&b, "claw_dispatch_duration_seconds_sum %f\n", c.durationSum)
	b.WriteString("# HELP claw_dispatch_duration_seconds_count Count backing claw_dispatch_duration_seconds_sum - divide sum/count for an average duration.\n")
	b.WriteString("# TYPE claw_dispatch_duration_seconds_count counter\n")
	fmt.Fprintf(&b, "claw_dispatch_duration_seconds_count %d\n", c.finishedCount)
	return b.String()
}

// Metrics returns a Prometheus text-exposition-format snapshot of every dispatch
// this Registry currently knows about.
func (reg *Registry) Metrics() string {
	reg.mu.Lock()
	all := make([]*dispatch, 0, len(reg.m))
	for _, d := range reg.m {
		all = append(all, d)
	}
	reg.mu.Unlock()

	states := make([]dispatchStatus, len(all))
	durations := make([]float64, len(all))
	for i, d := range all {
		d.mu.Lock()
		exited, exitCode, started, finished := d.exited, d.exitCode, d.startedAt, d.finished
		d.mu.Unlock()

		states[i] = classifyStatus(exited, exitCode)
		if states[i] != statusRunning {
			durations[i] = finished.Sub(started).Seconds()
		}
	}

	return aggregateCounts(states, durations).render()
}
