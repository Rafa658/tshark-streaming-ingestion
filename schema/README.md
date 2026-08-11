# Database Schema

## 001_init.sql

Source of truth for the TimescaleDB hypertable definition.

This script creates:
- `packets` hypertable with 8 columns (ts, src_ip, dst_ip, src_port, dst_port, proto, length, payload)
- btree index on (src_ip, dst_ip, ts) for talker lookback queries
- GIN index on payload (jsonb_path_ops) for nested field searches
- 1-hour chunk interval
- 1-day retention policy

## Applying changes

When you modify this file:
1. Run `make sync-schema` to copy it to docker/postgres/postgres-init/
2. For fresh DB: `docker compose -f docker/postgres/docker-compose.yml down -v && make up`
3. For existing DB: manual migration via `psql` (v1: tear-down + rebuild is acceptable)