// Package parity implements black-box verification of a tshark ingestion worker.
//
// It deliberately asserts only on observable outputs — rows in Postgres and the
// Prometheus /metrics endpoint — never on process internals. That keeps it valid
// against the Python worker and the Go worker alike, which is what lets it serve
// as the oracle for the port.
package parity

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// QuoteIdent validates and quotes a table identifier. Table names arrive from
// flags/env, and they cannot be passed as bind parameters, so they are validated
// against a strict allowlist before ever reaching a statement.
func QuoteIdent(name string) (string, error) {
	if !identRe.MatchString(name) {
		return "", fmt.Errorf("invalid identifier %q: must match %s", name, identRe)
	}
	return pgx.Identifier{name}.Sanitize(), nil
}

// Connect opens a pool against dsn.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return pool, nil
}

// CreateTableLike creates an empty table matching the packets schema. Used to give
// each worker its own destination table — running two QoS1 subscribers against the
// same table would duplicate every row, and v1 has no dedup.
func CreateTableLike(ctx context.Context, pool *pgxpool.Pool, table, like string) error {
	t, err := QuoteIdent(table)
	if err != nil {
		return err
	}
	l, err := QuoteIdent(like)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (LIKE %s INCLUDING DEFAULTS)", t, l))
	if err != nil {
		return fmt.Errorf("create table %s: %w", table, err)
	}
	return nil
}

// Truncate empties a table so a run starts from a known state.
func Truncate(ctx context.Context, pool *pgxpool.Pool, table string) error {
	t, err := QuoteIdent(table)
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "TRUNCATE "+t); err != nil {
		return fmt.Errorf("truncate %s: %w", table, err)
	}
	return nil
}

// RowCount returns the number of rows in table.
func RowCount(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	t, err := QuoteIdent(table)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

// Checksum is a content fingerprint of a table's packet rows.
type Checksum struct {
	Table    string
	Rows     int64
	Digest   string
	Payloads string
}

// ComputeChecksum fingerprints the semantic columns of a table.
//
// Rows are aggregated order-independently (sum of per-row md5) because MQTT QoS1
// gives no cross-batch ordering guarantee — two correct workers may commit the same
// rows in a different physical order. Sorting by ts alone is not stable either, since
// many packets share a millisecond timestamp.
//
// The payload digest is computed separately over the JSONB column so a mismatch tells
// you immediately whether the divergence is in the shaped columns or in raw payload
// preservation (the re-marshalling trap).
func ComputeChecksum(ctx context.Context, pool *pgxpool.Pool, table string) (Checksum, error) {
	t, err := QuoteIdent(table)
	if err != nil {
		return Checksum{}, err
	}

	const qTmpl = `
		SELECT
		  count(*),
		  coalesce(md5(sum(('x' || substr(md5(
		    coalesce(to_char(ts AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US'), '~') || '|' ||
		    coalesce(host(src_ip), '~')  || '|' ||
		    coalesce(host(dst_ip), '~')  || '|' ||
		    coalesce(src_port::text, '~') || '|' ||
		    coalesce(dst_port::text, '~') || '|' ||
		    coalesce(proto, '~')          || '|' ||
		    coalesce(length::text, '~')
		  ), 1, 8))::bit(32)::bigint)::text), 'empty'),
		  coalesce(md5(sum(('x' || substr(md5(payload::text), 1, 8))::bit(32)::bigint)::text), 'empty')
		FROM `
	q := qTmpl + t

	var c Checksum
	c.Table = table
	if err := pool.QueryRow(ctx, q).Scan(&c.Rows, &c.Digest, &c.Payloads); err != nil {
		return Checksum{}, fmt.Errorf("checksum %s: %w", table, err)
	}
	return c, nil
}

// WaitForDrain blocks until a table's row count stops growing for `stable`, or ctx ends.
// Returns the final count. A count that never moves at all is reported as an error,
// because that means nothing was ingested and a "parity pass" would be vacuous.
func WaitForDrain(ctx context.Context, pool *pgxpool.Pool, table string, stable, poll time.Duration) (int64, error) {
	var last int64 = -1
	var unchangedSince time.Time
	var everGrew bool

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return last, fmt.Errorf("timed out waiting for %s to drain (last count %d)", table, last)
		case <-ticker.C:
			n, err := RowCount(ctx, pool, table)
			if err != nil {
				return 0, err
			}
			if n != last {
				if n > 0 {
					everGrew = true
				}
				last = n
				unchangedSince = time.Now()
				continue
			}
			if !unchangedSince.IsZero() && time.Since(unchangedSince) >= stable {
				if !everGrew || last == 0 {
					return last, fmt.Errorf("table %s never received rows - nothing to verify", table)
				}
				return last, nil
			}
		}
	}
}

// DiffRows returns up to limit rows present in table a but not in table b, as a
// human-readable summary for debugging a checksum mismatch.
func DiffRows(ctx context.Context, pool *pgxpool.Pool, a, b string, limit int) ([]string, error) {
	ta, err := QuoteIdent(a)
	if err != nil {
		return nil, err
	}
	tb, err := QuoteIdent(b)
	if err != nil {
		return nil, err
	}

	const cols = "ts, src_ip, dst_ip, src_port, dst_port, proto, length"
	q := fmt.Sprintf(`
		SELECT %s FROM (
		  SELECT %s FROM %s EXCEPT ALL SELECT %s FROM %s
		) d LIMIT $1`, cols, cols, ta, cols, tb)

	rows, err := pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("diff %s vs %s: %w", a, b, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%v", vals))
	}
	return out, rows.Err()
}
