package parity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQuoteIdentRejectsInjection(t *testing.T) {
	bad := []string{
		"packets; DROP TABLE packets",
		"packets--",
		"pack ets",
		"",
		"1packets",
		`packets"`,
		"packets)",
	}
	for _, s := range bad {
		if _, err := QuoteIdent(s); err == nil {
			t.Errorf("QuoteIdent(%q) accepted a dangerous identifier", s)
		}
	}
}

func TestQuoteIdentAcceptsValid(t *testing.T) {
	for _, s := range []string{"packets", "packets_py", "_x", "P1"} {
		got, err := QuoteIdent(s)
		if err != nil {
			t.Errorf("QuoteIdent(%q) rejected a valid identifier: %v", s, err)
			continue
		}
		if !strings.Contains(got, s) {
			t.Errorf("QuoteIdent(%q) = %q, want it to contain the name", s, got)
		}
	}
}

const sampleMetrics = `# HELP worker_msgs_consumed_total Total MQTT messages consumed
# TYPE worker_msgs_consumed_total counter
worker_msgs_consumed_total{service="worker-postgres"} 1200.0
worker_rows_inserted_total{service="worker-postgres"} 1200.0
worker_batch_flush_total{service="worker-postgres"} 12.0
worker_deadletter_total{service="worker-postgres"} 0.0
worker_db_errors_total{service="worker-postgres"} 0.0
worker_buffer_size{service="worker-postgres"} 3.0
process_memory_bytes{service="worker-postgres"} 1.4680064e+07
`

func serve(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
}

func TestScrapeMetricsParsesLabelsAndBareNames(t *testing.T) {
	srv := serve(sampleMetrics)
	defer srv.Close()

	s, err := ScrapeMetrics(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Metrics[`worker_buffer_size{service="worker-postgres"}`]; got != 3 {
		t.Errorf("labelled series = %v, want 3", got)
	}
	if got := s.Metrics["worker_buffer_size"]; got != 3 {
		t.Errorf("bare name = %v, want 3", got)
	}
	if got := s.Metrics["process_memory_bytes"]; got != 14680064 {
		t.Errorf("scientific notation = %v, want 14680064", got)
	}
}

func TestScrapeMetricsSumsAcrossLabelSets(t *testing.T) {
	srv := serve(`process_memory_bytes{service="a"} 100
process_memory_bytes{service="b"} 250
`)
	defer srv.Close()

	s, err := ScrapeMetrics(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Metrics["process_memory_bytes"]; got != 350 {
		t.Errorf("bare name = %v, want 350 (sum across label sets)", got)
	}
}

func TestCheckRequiredMetricsDetectsDashboardBreakage(t *testing.T) {
	srv := serve(`worker_msgs_consumed_total{service="w"} 1
worker_rows_inserted_total{service="w"} 1
`)
	defer srv.Close()

	s, _ := ScrapeMetrics(context.Background(), srv.URL)
	missing := CheckRequiredMetrics(s)
	if len(missing) != 5 {
		t.Fatalf("missing = %v, want 5 entries", missing)
	}
	for _, want := range []string{"process_memory_bytes", "worker_buffer_size", "worker_deadletter_total"} {
		var found bool
		for _, m := range missing {
			if m == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s should be reported missing", want)
		}
	}
}

func TestCheckRequiredMetricsPassesOnFullSet(t *testing.T) {
	srv := serve(sampleMetrics)
	defer srv.Close()
	s, _ := ScrapeMetrics(context.Background(), srv.URL)
	if missing := CheckRequiredMetrics(s); len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

func TestScrapeMetricsErrorsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := ScrapeMetrics(context.Background(), srv.URL); err == nil {
		t.Error("expected error on 503")
	}
}

func TestSoakFailsWhenRSSExceedsGate(t *testing.T) {
	srv := serve(strings.Replace(sampleMetrics,
		"process_memory_bytes{service=\"worker-postgres\"} 1.4680064e+07",
		"process_memory_bytes{service=\"worker-postgres\"} 4.0e+07", 1))
	defer srv.Close()

	g := Gate{MaxRSSBytes: 20 * 1024 * 1024, MaxDeadLetters: 0, Duration: 60 * time.Millisecond}
	res, err := Soak(context.Background(), srv.URL, g, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed() {
		t.Error("gate should fail at 40 MiB RSS against a 20 MiB ceiling")
	}
	if !strings.Contains(res.String(), "peak RSS") {
		t.Errorf("failure reason should name RSS, got:\n%s", res)
	}
}

func TestSoakFailsOnDeadLetters(t *testing.T) {
	srv := serve(strings.Replace(sampleMetrics,
		`worker_deadletter_total{service="worker-postgres"} 0.0`,
		`worker_deadletter_total{service="worker-postgres"} 7.0`, 1))
	defer srv.Close()

	g := Gate{MaxRSSBytes: 20 * 1024 * 1024, MaxDeadLetters: 0, Duration: 60 * time.Millisecond}
	res, _ := Soak(context.Background(), srv.URL, g, 10*time.Millisecond)
	if res.Passed() {
		t.Error("gate should fail with 7 dead-letters")
	}
	if !strings.Contains(res.String(), "dead-letters") {
		t.Errorf("failure reason should name dead-letters, got:\n%s", res)
	}
}

func TestSoakFailsWhenNoRowsInserted(t *testing.T) {
	// static counter: rows never advance, so the load generator isn't running and a
	// "pass" would be meaningless
	srv := serve(sampleMetrics)
	defer srv.Close()

	g := Gate{MaxRSSBytes: 20 * 1024 * 1024, MaxDeadLetters: 0, Duration: 60 * time.Millisecond}
	res, _ := Soak(context.Background(), srv.URL, g, 10*time.Millisecond)
	if res.Passed() {
		t.Error("a soak with zero row growth must not pass")
	}
	if !strings.Contains(res.String(), "no rows inserted") {
		t.Errorf("failure reason should name the idle load generator, got:\n%s", res)
	}
}

func TestSoakPassesOnHealthyWorker(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		fmt.Fprintf(w, `worker_msgs_consumed_total{service="w"} %d
worker_rows_inserted_total{service="w"} %d
worker_batch_flush_total{service="w"} %d
worker_deadletter_total{service="w"} 0
worker_db_errors_total{service="w"} 0
worker_buffer_size{service="w"} %d
process_memory_bytes{service="w"} 1.4e+07
`, n*100, n*100, n, maxInt(0, 5-n))
	}))
	defer srv.Close()

	g := Gate{MaxRSSBytes: 20 * 1024 * 1024, MaxDeadLetters: 0, Duration: 120 * time.Millisecond}
	res, err := Soak(context.Background(), srv.URL, g, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed() {
		t.Errorf("healthy worker should pass, got:\n%s", res)
	}
}

func TestSoakFailsWhenBufferNeverDrains(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		fmt.Fprintf(w, `worker_msgs_consumed_total{service="w"} %d
worker_rows_inserted_total{service="w"} %d
worker_batch_flush_total{service="w"} %d
worker_deadletter_total{service="w"} 0
worker_db_errors_total{service="w"} 0
worker_buffer_size{service="w"} %d
process_memory_bytes{service="w"} 1.4e+07
`, n*100, n*100, n, n*10)
	}))
	defer srv.Close()

	g := Gate{MaxRSSBytes: 20 * 1024 * 1024, MaxDeadLetters: 0, Duration: 120 * time.Millisecond}
	res, _ := Soak(context.Background(), srv.URL, g, 10*time.Millisecond)
	if res.Passed() {
		t.Error("a monotonically growing buffer must fail the flat-buffer condition")
	}
	if !strings.Contains(res.String(), "buffer not flat") {
		t.Errorf("failure reason should name the buffer, got:\n%s", res)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
