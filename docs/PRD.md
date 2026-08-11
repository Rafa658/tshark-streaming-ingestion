# Tshark Streaming Ingestion — PRD v1.0 (Draft)

Date: 2026-08-09
Status: Draft

---

## 1. Overview & Problem Statement

Ingest tshark packet capture data as a streaming pipeline into a PostgreSQL instance with TimescaleDB installed (hypertable). The system routes streaming data through a Mosquitto MQTT broker; subscribers (now: Postgres worker; future: S3) receive messages and persist them independently. The architecture is agnostic to adding more subscribers — each is a separate MQTT client subscribing to the same topic.

## 2. Glossary

| Term | Definition |
|---|---|
| tshark | Network protocol analyzer, CLI version of Wireshark |
| MQTT | Lightweight pub/sub messaging protocol |
| Mosquitto | Open-source MQTT broker implementation |
| TimescaleDB | PostgreSQL extension for time-series data (hypertables, retention, continuous aggregates) |
| Hypertable | TimescaleDB partitioned table, auto-chunked by time |
| `-T ek` | tshark output format: Elastic bulk NDJSON (line-delimited JSON with `@timestamp`) |
| BPF | Berkeley Packet Filter — packet capture filter syntax (e.g. `tcp port 443`) |
| QoS | MQTT Quality of Service level (0=fire-and-forget, 1=at-least-once, 2=exactly-once) |
| Dead-letter | MQTT topic for malformed/unprocessable messages |
| Pcap | Packet capture file format |

## 3. Goals / Non-Goals

**Goals (v1):**
- Ingest live tshark capture (host-side on macOS for dev) into a TimescaleDB hypertable
- Support pcap file replay for testing and demos
- Pure MQTT fan-out architecture — adding S3 subscriber later = another MQTT client, no code change to ingestor
- Raw-data-first: store the full tshark ek line as JSONB for future flexibility
- At-least-once delivery (MQTT QoS1, accept duplicates on retry)
- Backpressure on Postgres failures (stop acking MQTT, buffer capped)
- 100 pps steady, 1-day retention, 1-hour chunk interval

**Non-Goals (v1):**
- MQTT authentication (TODO)
- TLS/encryption
- Deduplication beyond QoS1 retry semantics
- Kafka/other brokers
- Continuous aggregates (deferred)
- Prometheus/Grafana services (expose `/metrics` but no scraping)
- Tracing
- Schema migration tool (raw SQL only)

## 4. Architecture

### Topology

```
┌─────────────────┐         ┌──────────────────┐
│  tshark (host)  │         │  Mosquitto       │
│  -T ek -l       │ ──────► │  MQTT broker     │
│  stdout         │  1883   │  (no auth v1)    │
└─────────────────┘         └────────┬─────────┘
                                     │
                 ┌───────────────────┼───────────────────┐
                 │                   │                   │
                 ▼                   ▼                   ▼
        ┌────────────────┐   ┌──────────────┐   ┌────────────────┐
        │ worker-postgres│   │ (future:     │   │ (future:       │
        │ - mqtt sub     │   │  worker-s3)  │   │  other sinks)  │
        │ - batch 500/1s │   └──────────────┘   └────────────────┘
        │ - psycopg v3   │
        └───────┬────────┘
                │
                ▼
        ┌────────────────┐
        │ Postgres       │
        │ + TimescaleDB  │
        │ (hypertable)   │
        └────────────────┘
```

**Components (per service docker-compose folder):**
- `docker/mosquitto/` — Mosquitto 2, no auth v1, persistence on, listener 1883, external network `ingestion-net`
- `docker/postgres/` — TimescaleDB PostgreSQL 18 (pg18 pin), hypertable `packets`, 1h chunk, 1-day retention
- `docker/worker-postgres/` — Python 3.13.14-slim, paho-mqtt subscriber, psycopg v3, batch insert, backpressure
- `docker/ingestor-pcap/` — Python 3.13.14-slim, pcap replay mode, same code as host-side ingestor, `--source pcap --pcap /data/pcap/sample.pcap`

**Shared external network:** `ingestion-net` (created via `make network`). All services connect to it. Host-side ingestor connects to `localhost:1883` (Mosquitto port exposed to host).

**Orchestration:** root `Makefile` targets:
- `make network` — create `ingestion-net` if missing
- `make up` — bring up mosquitto, postgres, worker-postgres in order (worker handles retry-on-startup)
- `make up-replay` — same + ingestor-pcap
- `make down`, `make logs`, `make e2e`, `make test`, `make lint`

### Happy Path Sequence (ASCII)

```
[ingestor-host]            [mosquitto]            [worker]               [postgres]
     │                          │                       │                      │
     │ tshark -T ek -l          │                       │                      │
     │ line-by-line stdout      │                       │                      │
     ├─► subscribe topic        │                       │                      │
     ├─► publish topic (QoS1)──►│                       │                      │
     │                          │ deliver (QoS1)───────►│                      │
     │                          │                       │ batch buffer (deque) │
     │                          │                       │                      │
     │                          │                       │ (500 rows OR 1s)      │
     │                          │                       ├─────────► INSERT      │
     │                          │                       │ multi-row batch       │
     │                          │                       │                      │ ack batch
     │                          │ ack───────────────────┼──────────────────────►│
     │                          │                       │                      │
```

## 5. Components Spec

### 5.1 Ingestor (host-side live & containerized pcap)

**Purpose:** Shell out to tshark, read `-T ek` NDJSON line-by-line, publish to Mosquitto.

**Modes:**
- `--source live`: Host-side only (macOS dev). Runs `tshark -i <TSHARK_INTERFACE> -T ek -l [ -f "<TSHARK_BPF_FILTER>" ]`. Connects to `MQTT_HOST=localhost:MQTT_PORT`.
- `--source pcap`: Containerized. Runs `tshark -r <TSHARK_PCAP_PATH> -T ek`. Connects to `mosquitto:MQTT_PORT` via external network.

**Technology:** Python 3.13.14, `subprocess` for tshark, `paho-mqtt` publisher.

**MQTT topics:**
- `tshark/packets/v1` — canonical topic constant in `shared.mqtt` (single source of truth)
- `tshark/dead-letter/v1` — not used by ingestor; used by worker for parse failures

**Env vars (shared.config pydantic-settings):**
- `MQTT_HOST` (default `mosquitto` / `localhost` for host-side)
- `MQTT_PORT` (default `1883`)
- `MQTT_TOPIC_PACKETS` (default `tshark/packets/v1`)
- `MQTT_QOS` (default `1`)
- `MQTT_CLIENT_ID` (default `ingestor-1`)
- `LOG_LEVEL` (default `INFO`)

**Ingestor-specific:**
- `TSHARK_INTERFACE` (e.g. `en0`) — live mode only
- `TSHARK_BPF_FILTER` (optional) — live mode only, e.g. `tcp port 443`
- `TSHARK_PCAP_PATH` (e.g. `/data/pcap/sample.pcap`) — pcap mode only
- `TSHARK_REPLAY_LOOP` (bool, optional) — pcap mode only, loop the pcap file for continuous demo

**Error handling:**
- tshark process dies: log ERROR, exit (no restart logic; orchestration restarts container if needed)
- MQTT connection lost: paho auto-reconnects, unacked messages redelivered (QoS1)
- tshark output not valid ek line: log WARN, publish to `tshark/dead-letter/v1` as raw line with error annotation

**Logging:** `loguru`, JSON to stdout, fields: `ts`, `level`, `service=ingestor`, `event`, `count` (lines published), `latency_ms` (from tshark timestamp to publish timestamp).

**Metrics:** Expose `/metrics` on `:METRICS_PORT` (default `8000`) with counters:
- `ingestor_lines_read_total`
- `ingestor_msgs_published_total`
- `ingestor_parse_errors_total`

**Dockerfile (for pcap mode):**
- Base `python:3.13.14-slim`
- Install `tshark` (via apt) for pcap replay only; live mode uses host tshark
- Copy service code, install dependencies, expose `:8000/metrics`, entrypoint `python -m ingestor --source pcap --pcap /data/pcap/sample.pcap`

### 5.2 Mosquitto MQTT Broker

**Purpose:** Pub/sub broker, durable QoS1 sessions, persistence enabled.

**Configuration (`docker/mosquitto/mosquitto.conf`):**
```
listener 1883
allow_anonymous true
persistence true
persistence_location /mosquitto/data
max_queued_messages 50000
autosave_interval 1800
```

**Env vars:** None (config file only).

**Volumes:** `mosquitto-data` (persistence + queue state).

**Ports:** `1883` exposed to host (for host-side ingestor connection) and to `ingestion-net` (for services).

**Network:** External network `ingestion-net`.

**Healthcheck:** `mosquitto_sub -h localhost -t $SYS/broker/version -C 1` — exit 0 if OK.

### 5.3 Worker-Postgres

**Purpose:** Subscribe to `tshark/packets/v1`, shape ek line into hypertable row, batch insert into Postgres.

**Technology:** Python 3.13.14-slim, `paho-mqtt` (sync), `psycopg` v3 (sync, multi-row INSERT), `pydantic-settings`.

**Env vars (shared.config pydantic-settings):**
- `MQTT_HOST` (default `mosquitto`)
- `MQTT_PORT` (default `1883`)
- `MQTT_TOPIC_PACKETS` (default `tshark/packets/v1`)
- `MQTT_TOPIC_DEADLETTER` (default `tshark/dead-letter/v1`)
- `MQTT_QOS` (default `1`)
- `MQTT_CLIENT_ID` (default `worker-postgres-1`)
- `MQTT_SESSION_PERSISTENT` (default `true`) — QoS1 redelivery on reconnect
- `POSTGRES_DSN` (required, e.g. `postgresql://tshark_user:pass@postgres:5432/tshark_db`)
- `DB_BATCH_SIZE` (default `500`) — flush trigger
- `DB_BATCH_FLUSH_SECONDS` (default `1`) — flush trigger
- `DB_BUFFER_CAP` (default `50000`) — backpressure threshold
- `LOG_LEVEL` (default `INFO`)
- `METRICS_PORT` (default `8000`)

**Batching logic:**
- In-memory `deque` of ek lines
- Flush when: (len(deque) >= DB_BATCH_SIZE) OR (time since last flush >= DB_BATCH_FLUSH_SECONDS)
- Flush: multi-row `INSERT ... VALUES (...), (...)` via psycopg (prepared statement per worker startup)
- On flush success: ack MQTT messages (paho callback acks automatically on message receipt; we ACK only after successful DB insert by not stopping acks when buffer under cap)

**Backpressure:**
- DB down or connection error: stop acking MQTT messages (paho's `on_message` callback should not return until DB is ready or buffer cap reached)
- Buffer grows; when `len(deque) >= DB_BUFFER_CAP`: log ERROR, stop reading MQTT (paho `on_message` can raise to halt), optionally publish a backpressure message to a control topic
- DB reconnect: retry with exponential backoff, flush buffer once reconnected

**Dead-letter:**
- ek line fails to parse (missing `@timestamp` or other required fields): log WARN, publish raw line to `MQTT_TOPIC_DEADLETTER` with error annotation, continue processing next message

**Deduplication:** None v1 (accept duplicates on QoS1 retry). Document as known limitation.

**Startup ordering:**
- Worker tries to connect to Mosquitto and Postgres at startup
- Retry with exponential backoff until both are up
- No cross-file `depends_on` (services are in separate compose files)

**Error handling table:**

| Failure mode | Behavior |
|---|---|
| Mosquitto down at startup | Retry MQTT connection with backoff; log INFO each attempt |
| Postgres down at startup | Retry DB connection with backoff; log INFO each attempt |
| Mosquitto drops connection mid-stream | paho auto-reconnects; QoS1 redelivers unacked messages; buffer continues filling |
| Postgres drops connection mid-stream | Close DB conn, stop acking MQTT, buffer grows; on reconnect, flush buffer |
| Parse error on ek line | Log WARN, dead-letter to `tshark/dead-letter/v1`, continue |
| DB insert fails (constraint, etc.) | Log ERROR, dead-letter the batch, continue |
| Buffer cap exceeded | Log CRITICAL, stop acking MQTT (backpressure), optionally publish alert to control topic |

**Logging:** `loguru`, JSON to stdout, fields: `ts`, `level`, `service=worker-postgres`, `event`, `count` (rows inserted), `buffer_size`, `latency_ms` (from ek `@timestamp` to DB insert).

**Metrics:** Expose `/metrics` on `:METRICS_PORT` with counters/gauges:
- `worker_msgs_consumed_total`
- `worker_rows_inserted_total`
- `worker_batch_flush_total`
- `worker_deadletter_total`
- `worker_db_errors_total`
- `worker_buffer_size` (gauge)

**Dockerfile:**
- Base `python:3.13.14-slim`
- Copy service code + shared package, install dependencies
- Expose `:8000/metrics`
- Healthcheck: TCP check to mosquitto:1883 + `SELECT 1` to postgres via a tiny script

### 5.4 Postgres + TimescaleDB

**Purpose:** Persistent storage in a hypertable, auto-chunked, 1-day retention.

**Image:** `timescale/timescaledb:<latest-tag>-pg18` (verify tag at build time; if pg18 unavailable, fall back to latest pg17 and flag)

**Env vars:**
- `POSTGRES_USER` (default `tshark_user`)
- `POSTGRES_PASSWORD` (required, via .env)
- `POSTGRES_DB` (default `tshark_db`)
- `POSTGRES_INITDB_ARGS` (empty)

**Volumes:** `postgres-data` (DB data)

**Ports:** `5432:5432` (exposed to host for dev; also accessible on `ingestion-net`)

**Network:** External network `ingestion-net`.

**Initialization:** `docker/postgres/postgres-init/001-init.sql` mounted into `/docker-entrypoint-initdb.d/`. Contains hypertable DDL (see §6).

**Healthcheck:** `pg_isready -U tshark_user -d tshark_db`.

## 6. Data Model

### 6.1 Hypertable `packets`

**DDL (from `schema/001_init.sql`):**

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE packets (
    ts TIMESTAMPTZ NOT NULL,
    src_ip INET NOT NULL,
    dst_ip INET NOT NULL,
    src_port INTEGER,
    dst_port INTEGER,
    proto TEXT NOT NULL,
    length INTEGER NOT NULL,
    payload JSONB NOT NULL
);

SELECT create_hypertable('packets', 'ts', chunk_time_interval => INTERVAL '1 hour');

CREATE INDEX idx_packets_talker ON packets (src_ip, dst_ip, ts);
CREATE INDEX idx_packets_payload ON packets USING GIN (payload jsonb_path_ops);

-- 1-day retention policy
SELECT add_retention_policy('packets', INTERVAL '1 day');
```

**Column descriptions:**
- `ts` — `frame.time_epoch` from ek line, hypertable time column
- `src_ip` — `ip.src` or `ipv6.src` from ek
- `dst_ip` — `ip.dst` or `ipv6.dst` from ek
- `src_port` — `tcp.srcport` or `udp.srcport`; NULL for non-TCP/UDP
- `dst_port` — `tcp.dstport` or `udp.dstport`; NULL for non-TCP/UDP
- `proto` — full `frame.protocols` string from ek (e.g. `eth:ethertype:ip:tcp:http`)
- `length` — `frame.len` from ek
- `payload` — raw ek line as JSONB (parsed once at ingest; nested protocol data for future drill-down)

**Schema source of truth:** `schema/001_init.sql`. Copied into `docker/postgres/postgres-init/001-init.sql` via `make sync-schema`.

### 6.2 MQTT Topic Schema

**Topic: `tshark/packets/v1`**
- Payload: Raw tshark `-T ek` NDJSON line (one JSON object per line)
- QoS: 1 (at-least-once)
- Retain: false

**Topic: `tshark/dead-letter/v1`**
- Payload: JSON object with `{ raw_ek_line: string, error: string, ts: ISO8601 }`
- QoS: 1
- Retain: false

### 6.3 Ek Payload Schema

tshark `-T ek` outputs NDJSON like:
```json
{
  "@timestamp": "2026-08-09T12:34:56.789Z",
  "layers": {
    "frame": { "frame.len": 1234, "frame.protocols": "eth:ip:tcp:http" },
    "eth": { "eth.src": "00:11:22:33:44:55", "eth.dst": "aa:bb:cc:dd:ee:ff" },
    "ip": { "ip.src": "192.168.1.100", "ip.dst": "10.0.0.200", "ip.proto": "6" },
    "tcp": { "tcp.srcport": "443", "tcp.dstport": "54321", "tcp.flags": "0x002" },
    "http": { "http.request.uri": "/api/v1", "http.host": "example.com" }
  }
}
```

Worker parses:
- `ts` = `@timestamp`
- `src_ip` = `layers.ip.src` or `layers.ipv6.src`
- `dst_ip` = `layers.ip.dst` or `layers.ipv6.dst`
- `src_port` = `layers.tcp.srcport` or `layers.udp.srcport` (NULL if missing)
- `dst_port` = `layers.tcp.dstport` or `layers.udp.dstport` (NULL if missing)
- `proto` = `layers.frame.frame.protocols`
- `length` = `layers.frame.frame.len`
- `payload` = the entire ek JSON object

Parse error: any required field missing → dead-letter.

## 7. Scalability & Performance Assumptions

| Metric | Value | Notes |
|---|---|---|
| Peak packet rate | 100 pps steady | Sustained, not bursty |
| Record size | ~1 KB per ek line | Varies by packet complexity |
| Throughput on wire | ~100 KB/s peak | 100 pps × 1 KB |
| Daily row count | ~8.6M rows/day | 100 pps × 86400s |
| Daily DB size | ~8-9 GB/day | 8.6M rows × ~1KB (hypertable + indexes) |
| Chunk size | ~360k rows/chunk | 1 hour at 100 pps |
| Retention | 1 day | TimescaleDB `add_retention_policy` drops older chunks |
| Batch size threshold | 500 rows | Multi-row INSERT trigger |
| Batch flush interval | 1 second | Time trigger for flush |
| Buffer cap | 50,000 rows | Backpressure threshold; at 100 pps, ~500s of data (~8.3 min) |
| MQTT in-flight window | Default (not tuned) | paho-mqtt default; not a bottleneck at 100 pps |
| Postgres connections | 1 per worker | Single-thread worker, single DB connection |

At these assumptions, throughput is a non-issue. Design prioritizes simplicity over optimization.

## 8. Reliability & Failure Semantics

See §5.3 error handling table. Key properties:

- **At-least-once delivery:** MQTT QoS1 + worker persistence to DB. Duplicates possible on retry (no dedup v1).
- **Backpressure:** Worker stops acking MQTT when DB down or buffer cap reached. Mosquitto buffers unacked messages (subject to queue limit; configured at 50k). Ingestor can continue pushing; backpressure propagates.
- **Dead-letter:** Parse failures → worker publishes to `tshark/dead-letter/v1`. No silent drops.
- **Persistence:** Mosquitto persistence on; queue state survives restarts. Postgres persistent volume.
- **No data loss (v1):** Only if buffer cap exceeded and backpressure not honored; at 100 pps, 50k cap = 8.3 min of data, ample time to recover DB.

## 9. Extensibility

### 9.1 Adding S3 Subscriber

Add a new service `docker/worker-s3/` (Python container) that:
- Subscribes to `tshark/packets/v1` (same topic, same QoS)
- Processes ek lines (e.g. batch into Parquet files)
- Uploads to S3

No code change to ingestor. Mosquitto fan-out handles multiple subscribers.

### 9.2 Linux Prod Deployment Swap

For Linux servers, replace host-side ingestor with in-container capture:
- Use `network_mode: host` + `cap_add: [NET_ADMIN, NET_RAW]` in ingestor container
- Keep same `Source` abstraction; only impl changes
- tshark runs inside container with correct capabilities

Documented in PRD; code unchanged.

### 9.3 Future Broker Options

If durability/replay needed: add a Mosquitto bridge → Kafka. Keep Mosquitto for pub/sub edge; Kafka for log/backpressure. Kafka would replace the "messaging queue" layer explicitly (not added in v1 per Q1 decision).

## 10. Security

- **MQTT auth:** None v1. Mosquitto `allow_anonymous true`. Documented as TODO.
- **Postgres creds:** Username/password via `.env` (gitignored). No TLS v1 (localhost-only dev).
- **Network:** External network `ingestion-net` is a Docker bridge (not exposed to host except mosquitto port 1883 and postgres port 5432 for dev). No ACLs v1.
- **Secrets:** No vault/secret manager v1. Document as future work.

## 11. Observability

### Logging

- **Library:** `loguru`
- **Format:** JSON to stdout (container-friendly, parseable by docker logs)
- **Fields:** `ts`, `level`, `service`, `event`, `msg_id` (for dead-letter), `count`, `buffer_size`, `latency_ms`
- **Levels:**
  - DEBUG: line-by-line ingestor publish, worker batch flush stats every N
  - INFO: lifecycle (connect/disconnect/reconnect), batch flush summaries
  - WARN: parse errors, retries
  - ERROR: DB down, broker down, buffer near cap
  - CRITICAL: buffer cap exceeded, unrecoverable failures

### Metrics

- **Format:** Prometheus exposition (`# TYPE ...`, `# HELP ...`, `metric_name value`)
- **Endpoint:** `:METRICS_PORT/metrics` (default `:8000/metrics`) per service
- **Metrics per service:**
  - Ingestor: `ingestor_lines_read_total`, `ingestor_msgs_published_total`, `ingestor_parse_errors_total`
  - Worker-Postgres: `worker_msgs_consumed_total`, `worker_rows_inserted_total`, `worker_batch_flush_total`, `worker_deadletter_total`, `worker_db_errors_total`, `worker_buffer_size` (gauge)
- **Prometheus/Grafana:** Not deployed in v1. Services expose `/metrics`; scraping/grafana is future work.

### Tracing

- Skipped v1 (overkill at 100 pps single-host). Document as future work.

## 12. Testing Strategy

### Unit Tests

- **Framework:** `pytest`
- **Scope:** Pure functions
  - `services/worker-postgres/src/worker_postgres/shape.py` — ek line → row tuple
  - `services/worker-postgres/src/worker_postgres/batcher.py` — flush triggers
  - `packages/shared/src/tshark_shared/config.py` — env parsing
- **Location:** Per-service `tests/` directories
- **No external deps:** Fast, no containers

### Integration Tests

- **Framework:** `pytest` + `testcontainers-python`
- **Scope:** `worker-postgres` service with real Mosquitto + real Postgres+Timescale
  - Spin up Mosquitto container
  - Spin up TimescaleDB container
  - Publish sample ek lines via MQTT
  - Assert rows landed in Postgres with correct columns
- **Ingestor integration:** Mock MQTT client (since live capture requires host tshark); assert it publishes correct topic/payload for a canned ek line
- **Coverage:** Report via `pytest-cov`; no gate (v1)

### End-to-End Tests

- **Implementation:** `make e2e` Makefile target (not pytest)
- **Flow:**
  1. `make network` (ensure `ingestion-net` exists)
  2. `docker compose -f docker/mosquitto/docker-compose.yml -f docker/postgres/docker-compose.yml -f docker/worker-postgres/docker-compose.yml up -d`
  3. Wait for healthchecks
  4. `docker compose -f docker/ingestor-pcap/docker-compose.yml --profile replay up -d` (or use a small pcap mounted)
  5. Wait for pcap replay to complete
  6. Query Postgres via `psql`: `SELECT COUNT(*) FROM packets;` → should equal packet count in pcap (±dups)
  7. `make down`
- **Acceptance:** Part of §16 acceptance criteria

### Lint & Type

- **Linter:** `ruff` (config in root `pyproject.toml`)
- **Type checker:** `mypy` (config in root `pyproject.toml`)
- **Make target:** `make lint` runs both
- **CI:** Not wired v1 (no CI configured in repo). Ready to wire to GitHub Actions or similar.

## 13. Deployment

### Local Dev (macOS)

1. `make network` — create `ingestion-net`
2. `make up` — start mosquitto, postgres, worker-postgres
3. In a separate terminal (Poetry): `poetry run python -m ingestor --source live --interface en0` (capture from en0, publish to localhost:1883)
4. Observe logs: `docker compose -f docker/mosquitto/docker-compose.yml logs -f`
5. Query Postgres: `psql -h localhost -U tshark_user -d tshark_db -c "SELECT COUNT(*) FROM packets;"`

### Pcap Replay Demo

1. `make network` (if not done)
2. `make up-replay` — includes `ingestor-pcap` which replays `data/pcap/sample.pcap`
3. Same observation/query steps

### Production Deployment (Linux)

- Swap host-side ingestor for in-container capture:
  - Use `docker/ingestor/` compose (add live-mode service with `network_mode: host` + `cap_add: [NET_ADMIN, NET_RAW]`)
  - Same codebase, different `Source` impl
  - Orchestration via k8s or Docker Swarm; not specified v1

### Split-Compose Management

- Each service has its own `docker/<service>/docker-compose.yml`
- Orchestration via root `Makefile`:
  - `make network`: `docker network create ingestion-net`
  - `make up`: sequential `docker compose up -d` for mosquitto, postgres, worker-postgres
  - `make up-replay`: same + ingestor-pcap
  - `make down`: `docker compose down` for each
  - `make logs`: aggregate logs via `docker compose logs`
  - `make e2e`: full integration test sequence
  - `make test`: run unit + integration tests
  - `make lint`: `ruff check . && mypy .`

## 14. Repo Layout

```
tshark-streaming-ingestion/
├── services/
│   ├── ingestor/
│   │   ├── src/ingestor/
│   │   │   ├── __init__.py
│   │   │   ├── __main__.py       # CLI entry: --source live|pcap
│   │   │   ├── sources.py        # LiveTsharkSource, PcapReplaySource
│   │   │   └── publisher.py      # MQTT publish wrapper
│   │   ├── tests/
│   │   ├── Dockerfile
│   │   └── pyproject.toml
│   └── worker-postgres/
│       ├── src/worker_postgres/
│       │   ├── __init__.py
│       │   ├── __main__.py
│       │   ├── consumer.py       # paho-mqtt subscriber
│       │   ├── batcher.py        # flush-at-500-or-1s deque
│       │   ├── db.py             # psycopg multi-row INSERT
│       │   └── shape.py          # ek line -> row tuple
│       ├── tests/
│       ├── Dockerfile
│       └── pyproject.toml
├── packages/
│   └── shared/
│       ├── src/tshark_shared/
│       │   ├── __init__.py
│       │   ├── config.py         # pydantic settings
│       │   ├── mqtt.py           # client factory, topic constants
│       │   └── models.py         # PacketRow dataclass
│       └── pyproject.toml
├── schema/
│   ├── 001_init.sql              # hypertable DDL, indexes, retention
│   └── README.md                 # how to apply via make sync-schema
├── docker/
│   ├── mosquitto/
│   │   ├── docker-compose.yml
│   │   └── mosquitto.conf
│   ├── postgres/
│   │   ├── docker-compose.yml
│   │   └── postgres-init/
│   │       └── 001-init.sql      # copy of schema/001_init.sql
│   ├── worker-postgres/
│   │   └── docker-compose.yml
│   └── ingestor-pcap/
│       └── docker-compose.yml    # replay profile
├── data/
│   └── pcap/
│       └── sample.pcap           # small sample for replay (or fetched)
├── docs/
│   └── PRD.md                    # this document
├── .gitignore
├── Makefile                      # orchestration: network/up/down/logs/e2e/test/lint/migrate/sync-schema
├── pyproject.toml                # workspace root: dev deps, ruff, mypy
├── README.md                     # entry point doc (diagram + architecture choices + quickstart)
└── LICENSE                       # omitted v1 (not specified)
```

## 15. Open Questions / Future Work

- **MQTT authentication:** Add username/password to Mosquitto and services; update PRD §10
- **TLS/encryption:** MQTT over TLS, Postgres over TLS
- **Deduplication:** Add a `msg_id` column (hash of 5-tuple+ts) with unique index to reject duplicates
- **Kafka bridge:** Add Kafka for durability/replay if backpressure at scale becomes an issue
- **Continuous aggregate:** Create per-minute pps/top-talkers via TimescaleDB `create_continuous_aggregate` for faster queries
- **App-layer columns:** Promote `http.host`, `dns.qry.name`, etc. to first-class columns if queries become hot
- **Prometheus/Grafana:** Add scraping services + dashboards
- **Schema migration tool:** Adopt `dbmate` or Alembic once we have a second migration or prod data
- **Tracing:** Add OpenTelemetry for distributed tracing (if multi-service)
- **CI/CD:** Wire GitHub Actions or GitLab CI for test/lint/e2e on PRs
- **k8s deployment:** Write Helm charts or k8s manifests for prod deployment

## 16. Acceptance Criteria

v1 is done when:

1. **Live capture works:**
   - Host-side ingestor captures from `en0` (or specified iface) using `tshark -T ek -l`
   - Packets are published to `tshark/packets/v1` (Mosquitto confirms)
   - Worker-postgres subscribes, batches, and inserts rows into `packets` hypertable within 1s of capture

2. **Pcap replay works:**
   - `make up-replay` starts mosquitto, postgres, worker-postgres, and ingestor-pcap
   - Ingestor-pcap replays `data/pcap/sample.pcap` (or a fetched sample)
   - Postgres `SELECT COUNT(*) FROM packets;` equals the packet count in the pcap (±dups from QoS1 retry)

3. **Data model correct:**
   - Hypertable `packets` has all 8 columns (ts, src_ip, dst_ip, src_port, dst_port, proto, length, payload)
   - Indexes exist: btree `(src_ip,dst_ip,ts)`, GIN `payload jsonb_path_ops`
   - Chunk interval = 1 hour
   - Retention policy = 1 day
   - Payload column contains the full ek JSON object

4. **Failure handling:**
   - Mosquitto down at startup: worker retries, connects when broker up
   - Postgres down at startup: worker retries, connects when DB up
   - Mosquitto drops mid-stream: worker reconnects, unacked messages redelivered
   - Postgres drops mid-stream: worker stops acking, buffer grows; on reconnect, buffer flushes
   - Parse error: dead-letter published to `tshark/dead-letter/v1`, worker continues
   - Buffer cap exceeded: worker stops acking, logs CRITICAL

5. **Backpressure works:**
   - Simulate Postgres down: worker stops acking, Mosquitto queues messages (up to 50k), ingestor can continue
   - When Postgres back up, worker flushes buffer, ack resumes

6. **Observability:**
   - All services emit JSON logs to stdout (parseable by `docker logs`)
   - All services expose `:8000/metrics` with Prometheus counters/gauges
   - Metrics include the counters/gauges listed in §11

7. **Tests pass:**
   - `make test` runs unit tests (all pass)
   - `make test` runs integration tests (mosquitto + timescaledb via testcontainers, all pass)
   - `make lint` passes (ruff + mypy)

8. **E2E test passes:**
   - `make e2e` completes with Postgres count ≈ packet count

9. **Documentation:**
   - `README.md` exists with diagram + architecture choices + quickstart
   - `docs/PRD.md` exists with all 18 sections, self-contained

10. **Monorepo layout exists:**
    - All directories from §14 present
    - Each service has `Dockerfile`, `pyproject.toml`, `tests/`
    - `packages/shared` installed editable in dev, copied into images at build

## 17. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Mosquitto auth missing → unauthorized access on prod | Low (v1 dev only) | High | Document as TODO; require auth before prod deployment |
| No dedup → duplicate rows on retry | High (by design v1) | Medium | Document as known limitation; v2 add msg_id hash column |
| Buffer cap exceeded → data loss on prolonged outage | Low (100 pps, 50k cap = 8.3 min) | High | Monitor buffer size metrics; alert when > 75%; increase cap if needed |
| tshark -T ek format changes | Low (Wireshark stable) | High | Pin tshark version in Dockerfile; test against specific ek schema |
| pg18 Timescale image unavailable | Medium (new version) | Medium | Fallback to pg17 if pg18 tag missing; flag in PRD |
| macOS Docker Desktop network quirks for live capture | High (known issue) | Medium | Avoid by running ingestor host-side (v1 approach); document Linux prod swap path |
| Mosquitto queue limit hit (50k) → message drop | Low (at 100 pps) | Medium | Monitor queue metrics; tune `max_queued_messages` if needed |

## 18. References

- tshark output formats: [Wireshark `-T ek` docs](https://www.wireshark.org/docs/man-pages/tshark.html)
- TimescaleDB hypertables: [TimescaleDB documentation](https://docs.timescale.com/)
- paho-mqtt Python: [paho-mqtt GitHub](https://github.com/eclipse/paho.mqtt.python)
- psycopg 3: [psycopg documentation](https://www.psycopg.org/psycopg3/docs/)
- loguru: [loguru documentation](https://loguru.readthedocs.io/)
- pydantic-settings: [pydantic-settings documentation](https://docs.pydantic.dev/latest/concepts/pydantic_settings/)
- testcontainers-python: [testcontainers documentation](https://testcontainers-python.readthedocs.io/)
- ruff: [ruff documentation](https://docs.astral.sh/ruff/)
- mypy: [mypy documentation](https://mypy.readthedocs.io/)
