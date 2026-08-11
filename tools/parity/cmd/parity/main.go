// Command parity is the black-box e2e / parity harness for the tshark ingestion
// pipeline. It verifies a worker implementation purely through its observable
// outputs, so the same binary validates the Python worker and the Go worker.
//
//	parity verify  -a packets_py -b packets_go     compare two workers' output
//	parity soak    -metrics URL                    run the Stage 1 RSS gate
//	parity drain   -table packets                  wait until ingestion settles
//	parity metrics -metrics URL                    check Grafana metric contract
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rafael/tshark-streaming-ingestion/tools/parity/internal/parity"
)

const defaultDSN = "postgres://tshark_user:tshark_password@localhost:5432/tshark_db"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "verify":
		err = runVerify(ctx, os.Args[2:])
	case "soak":
		err = runSoak(ctx, os.Args[2:])
	case "drain":
		err = runDrain(ctx, os.Args[2:])
	case "metrics":
		err = runMetrics(ctx, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nERROR: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `parity - black-box e2e/parity harness for tshark ingestion

  parity verify   -a TABLE -b TABLE [-dsn DSN] [-diff N]
      Compare two workers' output tables by content checksum.
      Exits non-zero on any divergence in shaped columns or raw payload.

  parity soak     -metrics URL [-duration 30m] [-max-rss-mib 20] [-poll 5s]
      Poll a worker's /metrics for the gate duration and evaluate the
      Stage 1 criterion (RSS ceiling, zero dead-letters, flat buffer).

  parity drain    -table TABLE [-dsn DSN] [-stable 10s] [-timeout 5m]
      Block until a table's row count stops growing.

  parity metrics  -metrics URL
      Assert the worker emits every series the Grafana dashboard queries.
`)
}

func runVerify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	dsn := fs.String("dsn", envOr("POSTGRES_DSN", defaultDSN), "postgres DSN")
	a := fs.String("a", "", "first table (reference)")
	b := fs.String("b", "", "second table (candidate)")
	diffN := fs.Int("diff", 5, "sample rows to show on mismatch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *a == "" || *b == "" {
		return fmt.Errorf("both -a and -b are required")
	}

	pool, err := parity.Connect(ctx, *dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	ca, err := parity.ComputeChecksum(ctx, pool, *a)
	if err != nil {
		return err
	}
	cb, err := parity.ComputeChecksum(ctx, pool, *b)
	if err != nil {
		return err
	}

	fmt.Printf("%-24s rows=%-8d digest=%s payload=%s\n", ca.Table, ca.Rows, ca.Digest, ca.Payloads)
	fmt.Printf("%-24s rows=%-8d digest=%s payload=%s\n", cb.Table, cb.Rows, cb.Digest, cb.Payloads)

	if ca.Rows == 0 || cb.Rows == 0 {
		return fmt.Errorf("refusing to pass on an empty table (%s=%d, %s=%d): "+
			"a vacuous match proves nothing", ca.Table, ca.Rows, cb.Table, cb.Rows)
	}

	var problems []string
	if ca.Rows != cb.Rows {
		problems = append(problems, fmt.Sprintf("row count %d != %d", ca.Rows, cb.Rows))
	}
	if ca.Digest != cb.Digest {
		problems = append(problems, "shaped-column digest mismatch")
	}
	if ca.Payloads != cb.Payloads {
		problems = append(problems, "payload JSONB digest mismatch "+
			"(likely a re-marshalling difference - payload must pass through verbatim)")
	}

	if len(problems) == 0 {
		fmt.Printf("\nPARITY OK - %d rows identical across %s and %s\n", ca.Rows, ca.Table, cb.Table)
		return nil
	}

	fmt.Printf("\nPARITY FAILED:\n")
	for _, p := range problems {
		fmt.Printf("  - %s\n", p)
	}
	if rows, derr := parity.DiffRows(ctx, pool, *a, *b, *diffN); derr == nil && len(rows) > 0 {
		fmt.Printf("\n  rows in %s missing from %s:\n", *a, *b)
		for _, r := range rows {
			fmt.Printf("    %s\n", r)
		}
	}
	return fmt.Errorf("parity check failed")
}

func runSoak(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("soak", flag.ExitOnError)
	url := fs.String("metrics", "http://localhost:8000/metrics", "worker metrics endpoint")
	dur := fs.Duration("duration", 30*time.Minute, "soak duration")
	maxRSS := fs.Float64("max-rss-mib", 20, "RSS ceiling in MiB")
	poll := fs.Duration("poll", 5*time.Second, "scrape interval")
	if err := fs.Parse(args); err != nil {
		return err
	}

	g := parity.DefaultGate()
	g.Duration = *dur
	g.MaxRSSBytes = *maxRSS * 1024 * 1024

	fmt.Printf("soaking %s for %s (RSS ceiling %.0f MiB)\n", *url, *dur, *maxRSS)
	res, err := parity.Soak(ctx, *url, g, *poll)
	if err != nil {
		return err
	}
	fmt.Print(res)
	if !res.Passed() {
		return fmt.Errorf("gate failed")
	}
	return nil
}

func runDrain(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("drain", flag.ExitOnError)
	dsn := fs.String("dsn", envOr("POSTGRES_DSN", defaultDSN), "postgres DSN")
	table := fs.String("table", "packets", "table to watch")
	stable := fs.Duration("stable", 10*time.Second, "row count must hold steady this long")
	timeout := fs.Duration("timeout", 5*time.Minute, "give up after this long")
	poll := fs.Duration("poll", time.Second, "poll interval")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pool, err := parity.Connect(ctx, *dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	tctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	n, err := parity.WaitForDrain(tctx, pool, *table, *stable, *poll)
	if err != nil {
		return err
	}
	fmt.Printf("drained: %s settled at %d rows\n", *table, n)
	return nil
}

func runMetrics(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	url := fs.String("metrics", "http://localhost:8000/metrics", "worker metrics endpoint")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := parity.ScrapeMetrics(ctx, *url)
	if err != nil {
		return err
	}
	missing := parity.CheckRequiredMetrics(s)
	for _, m := range parity.RequiredMetrics {
		if v, ok := s.Metrics[m]; ok {
			fmt.Printf("  ok      %-32s %.0f\n", m, v)
		} else {
			fmt.Printf("  MISSING %-32s\n", m)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d dashboard metric(s) missing - Grafana panels would go blank", len(missing))
	}
	fmt.Println("\nmetric contract satisfied")
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
