package parity

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sample is one observation of a metrics endpoint.
type Sample struct {
	At      time.Time
	Metrics map[string]float64
}

// ScrapeMetrics fetches a Prometheus text-format endpoint and flattens it.
//
// Series are keyed by name plus sorted labels (e.g. `worker_buffer_size{service="x"}`)
// and additionally accumulated under the bare metric name, so callers can read a
// metric without knowing its label set.
func ScrapeMetrics(ctx context.Context, url string) (Sample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Sample{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Sample{}, fmt.Errorf("scrape %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Sample{}, fmt.Errorf("scrape %s: status %d", url, resp.StatusCode)
	}

	s := Sample{At: time.Now(), Metrics: map[string]float64{}}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i < 0 {
			continue
		}
		series, raw := line[:i], line[i+1:]
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		s.Metrics[series] = v

		// also accumulate under the bare name, summing across label sets
		if j := strings.IndexByte(series, '{'); j > 0 {
			s.Metrics[series[:j]] += v
		}
	}
	if err := sc.Err(); err != nil {
		return Sample{}, err
	}
	return s, nil
}

// RequiredMetrics are the series the Grafana dashboard queries. The Go worker must
// emit every one of these or docker/grafana/dashboards/tshark.json silently goes blank.
var RequiredMetrics = []string{
	"worker_msgs_consumed_total",
	"worker_rows_inserted_total",
	"worker_batch_flush_total",
	"worker_deadletter_total",
	"worker_db_errors_total",
	"worker_buffer_size",
	"process_memory_bytes",
}

// CheckRequiredMetrics reports which dashboard-contract metrics are absent.
func CheckRequiredMetrics(s Sample) []string {
	var missing []string
	for _, m := range RequiredMetrics {
		if _, ok := s.Metrics[m]; !ok {
			missing = append(missing, m)
		}
	}
	sort.Strings(missing)
	return missing
}

// Gate is the Stage 1 pass/fail criterion agreed during planning.
type Gate struct {
	MaxRSSBytes    float64
	MaxDeadLetters float64
	Duration       time.Duration
}

// DefaultGate is the locked criterion: under 20 MiB RSS, zero dead-letters, flat
// buffer, sustained for 30 minutes at 100 pps.
func DefaultGate() Gate {
	return Gate{
		MaxRSSBytes:    20 * 1024 * 1024,
		MaxDeadLetters: 0,
		Duration:       30 * time.Minute,
	}
}

// SoakResult summarises a soak run.
type SoakResult struct {
	Samples      int
	PeakRSS      float64
	FinalRSS     float64
	FirstRSS     float64
	DeadLetters  float64
	DBErrors     float64
	PeakBuffer   float64
	FinalBuffer  float64
	RowsInserted float64
	MsgsConsumed float64
	Elapsed      time.Duration
	Missing      []string
	Failures     []string
}

// Passed reports whether every gate condition held.
func (r SoakResult) Passed() bool { return len(r.Failures) == 0 && len(r.Missing) == 0 }

func (r SoakResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "soak: %s, %d samples\n", r.Elapsed.Round(time.Second), r.Samples)
	fmt.Fprintf(&b, "  RSS         first=%s peak=%s final=%s\n",
		humanBytes(r.FirstRSS), humanBytes(r.PeakRSS), humanBytes(r.FinalRSS))
	fmt.Fprintf(&b, "  buffer      peak=%.0f final=%.0f\n", r.PeakBuffer, r.FinalBuffer)
	fmt.Fprintf(&b, "  consumed    %.0f msgs -> %.0f rows\n", r.MsgsConsumed, r.RowsInserted)
	fmt.Fprintf(&b, "  deadletters %.0f, db errors %.0f\n", r.DeadLetters, r.DBErrors)
	if r.Elapsed > 0 {
		fmt.Fprintf(&b, "  throughput  %.1f rows/sec\n", r.RowsInserted/r.Elapsed.Seconds())
	}
	for _, m := range r.Missing {
		fmt.Fprintf(&b, "  MISSING METRIC (breaks Grafana): %s\n", m)
	}
	for _, f := range r.Failures {
		fmt.Fprintf(&b, "  FAIL: %s\n", f)
	}
	if r.Passed() {
		b.WriteString("  GATE PASSED\n")
	}
	return b.String()
}

func humanBytes(b float64) string {
	return fmt.Sprintf("%.1f MiB", b/1024/1024)
}

// Soak polls a metrics endpoint for the gate duration and evaluates the criterion.
func Soak(ctx context.Context, url string, g Gate, poll time.Duration) (SoakResult, error) {
	var r SoakResult
	start := time.Now()
	deadline := start.Add(g.Duration)

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	// baseline scrape up front so a totally dead endpoint fails fast rather than
	// after the full soak duration
	first, err := ScrapeMetrics(ctx, url)
	if err != nil {
		return r, err
	}
	r.Missing = CheckRequiredMetrics(first)
	r.FirstRSS = first.Metrics["process_memory_bytes"]
	baselineRows := first.Metrics["worker_rows_inserted_total"]

	apply := func(s Sample) {
		r.Samples++
		rss := s.Metrics["process_memory_bytes"]
		if rss > r.PeakRSS {
			r.PeakRSS = rss
		}
		r.FinalRSS = rss
		buf := s.Metrics["worker_buffer_size"]
		if buf > r.PeakBuffer {
			r.PeakBuffer = buf
		}
		r.FinalBuffer = buf
		r.DeadLetters = s.Metrics["worker_deadletter_total"]
		r.DBErrors = s.Metrics["worker_db_errors_total"]
		r.RowsInserted = s.Metrics["worker_rows_inserted_total"] - baselineRows
		r.MsgsConsumed = s.Metrics["worker_msgs_consumed_total"]
	}
	apply(first)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-ticker.C:
			s, err := ScrapeMetrics(ctx, url)
			if err != nil {
				return r, err
			}
			apply(s)
		}
	}
	r.Elapsed = time.Since(start)

	if r.PeakRSS > g.MaxRSSBytes {
		r.Failures = append(r.Failures, fmt.Sprintf(
			"peak RSS %s exceeds limit %s", humanBytes(r.PeakRSS), humanBytes(g.MaxRSSBytes)))
	}
	if r.DeadLetters > g.MaxDeadLetters {
		r.Failures = append(r.Failures, fmt.Sprintf(
			"%.0f dead-letters, limit %.0f", r.DeadLetters, g.MaxDeadLetters))
	}
	// "flat buffer" = ends where it started, not merely bounded; a buffer that ends
	// high is still draining slower than it fills.
	if r.FinalBuffer > 0 && r.FinalBuffer >= r.PeakBuffer {
		r.Failures = append(r.Failures, fmt.Sprintf(
			"buffer not flat: final %.0f >= peak %.0f (worker is falling behind)",
			r.FinalBuffer, r.PeakBuffer))
	}
	if r.RowsInserted == 0 {
		r.Failures = append(r.Failures, "no rows inserted during soak - load generator not running?")
	}
	return r, nil
}
