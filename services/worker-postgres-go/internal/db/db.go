package db

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/shape"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// InsertBatch inserts rows into the given table using a pipelined batch.
// Table names arrive from env and cannot be parameterised, so they are
// validated against a strict identifier allowlist before composition.
func InsertBatch(ctx context.Context, pool *pgxpool.Pool, rows []shape.Row, table string) error {
	if !identRe.MatchString(table) {
		return fmt.Errorf("invalid table name: %q", table)
	}
	query := fmt.Sprintf(`
		INSERT INTO %s (ts, src_ip, dst_ip, src_port, dst_port, proto, length, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, pgx.Identifier{table}.Sanitize())

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(query, r.TS, r.SrcIP.String(), r.DstIP.String(),
			r.SrcPort, r.DstPort, r.Proto, r.Length, r.Payload)
	}
	br := pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
	}
	return nil
}
