# Tshark Streaming Ingestion

Streaming pipeline that ingests tshark packet capture data via MQTT into a PostgreSQL instance with TimescaleDB (hypertable).

## Architecture

```
tshark (host) ──► ingestor (Python) ──► Mosquitto MQTT broker (no auth v1)
                                            │
                                            ├─► worker-postgres (Go) ──► PostgreSQL + TimescaleDB (hypertable)
                                            │
                                            └─► (future: worker-s3, etc. each subscribe to same topic)
```

`worker-postgres` was ported from Python to Go to cut its steady-state RSS footprint;
`ingestor` remains Python since it's mostly `tshark` subprocess plumbing and must run on
the host for live capture. The port was staged behind a measured gate — see `docs/PRD.md`
for the throughput assumptions (100 pps) that framed the decision, and the git history
(`feat(worker-postgres-go)`, `feat: cutover to Go worker-postgres`) for how it was validated.

### Key Architecture Choices

- **Pure MQTT fan-out** — adding S3 subscriber = another MQTT client, no code change to ingestor
- **Raw-data-first** — store full tshark `-T ek` line as JSONB (`payload` column), transform later
- **Host-side ingestor for live capture** (macOS dev, Python), containerized pcap replay; Linux prod swap path documented
- **Go worker for steady-state footprint** — `worker-postgres` is a static Go binary on a
  `scratch` base image (`pgx/v5` + `paho.mqtt.golang` + `client_golang`), capped at `GOMEMLIMIT=18MiB`
- **Backpressure on DB failures and buffer cap** — stop acking MQTT, buffer capped at 50k rows
- **Split-compose** — one service per docker-compose folder, shared external network, root Makefile orchestration
- **At-least-once delivery** — MQTT QoS1, accept duplicates on retry (no dedup v1)
- **Observability** — JSON logging, `/metrics` exposed per service via Prometheus + Grafana

## Quickstart

### Prerequisites

- macOS dev: `tshark` installed via Homebrew (`brew install tshark`)
- Docker & Docker Compose
- Poetry (`curl -sSL https://install.python-poetry.org | python3 -`) — for `ingestor` and shared tooling
- Python 3.13.14
- Go 1.26+ — only needed if building/modifying `worker-postgres` outside Docker (the
  compose build stage installs its own toolchain in-container)

### Setup

```bash
# Clone and navigate
git clone <repo>
cd tshark-streaming-ingestion

# Install Python packages with Poetry (ingestor + shared)
make install

# Install dev dependencies (for testing/linting)
make dev

# Create shared Docker network
make network

# Start the stack (mosquitto, postgres, worker-postgres, prometheus, grafana)
make up

# In another terminal, run live capture on the host (sudo needed for tshark BPF access)
make live IFACE=en0

# Or run pcap replay demo instead of live capture
make up-replay
```

### Verify

```bash
# Check logs
docker compose -f docker/mosquitto/docker-compose.yml logs -f
docker compose -f docker/postgres/docker-compose.yml logs -f
docker compose -f docker/worker-postgres/docker-compose.yml logs -f

# Query Postgres
psql -h localhost -U tshark_user -d tshark_db -c "SELECT COUNT(*) FROM packets;"

# worker-postgres metrics (Go, published on the host)
curl -s localhost:8001/metrics | grep worker_
```

## Metrics & Dashboards

Both services expose Prometheus metrics on `/metrics`; Prometheus scrapes them and
Grafana renders the `tshark-streaming` dashboard on top.

| Where | URL | Notes |
|---|---|---|
| Prometheus | http://localhost:9090 | Raw metrics, ad-hoc PromQL queries, scrape target status |
| Grafana | http://localhost:3030 | Dashboard: **Tshark Streaming Ingestion** (`admin` / `admin`, local dev only) |
| `worker-postgres` `/metrics` | http://localhost:8001/metrics | Go worker, MQTT→Postgres |
| `ingestor` `/metrics` | http://localhost:8002/metrics | Python ingestor (pcap replay or `make live`) |

Key series (also what the dashboard's Grafana panels query — renaming any of these
breaks the panels):

| Metric | What it means |
|---|---|
| `ingestor_msgs_published_total` | Packets published to MQTT by the ingestor |
| `worker_msgs_consumed_total` | Packets consumed off MQTT by the worker |
| `worker_rows_inserted_total` | Rows successfully batch-inserted into Postgres |
| `worker_batch_flush_total` | Number of batch flushes to the DB |
| `worker_db_errors_total` | Failed DB writes (retried with backoff before counting) |
| `worker_deadletter_total` | Unparseable ek lines published to the dead-letter topic |
| `worker_buffer_size` | Current in-memory buffer depth (backpressure engages at `DB_BUFFER_CAP`) |
| `process_memory_bytes{service=...}` | RSS per service — the number the Go port was measured against |

`make observability` starts Prometheus + Grafana + cadvisor standalone if you only want
the dashboards without the full ingestion stack; `make obs-down` stops them.

### Run Tests

```bash
make test          # Python unit tests (Poetry pytest) - ingestor + shared
make lint          # ruff + mypy (Poetry)
make e2e           # full end-to-end with pcap replay

# Go worker tests
cd services/worker-postgres && go test ./...
```

### Teardown

```bash
make down          # stop all services
```

## Poetry Workspace (Python services)

`ingestor` and `packages/shared` remain a Poetry workspace. `worker-postgres` is a
standalone Go module (`services/worker-postgres/go.mod`) built via its own Dockerfile —
it is not part of the Poetry workspace.

- `packages/shared` — shared utilities and configuration
- `services/ingestor` — tshark to MQTT ingestor

Common Poetry commands:
```bash
poetry install              # Install all dependencies
poetry install --with dev   # Include dev dependencies
poetry lock                 # Update lock file
poetry run python -m ...    # Run Python modules
poetry run pytest           # Run tests
poetry run ruff check .     # Lint
poetry run mypy .           # Type check
```

Common Go commands (from `services/worker-postgres`):
```bash
go build ./...              # Build
go test ./...               # Run tests
go vet ./...                # Static checks
```

## Commands

See [`COMMANDS.md`](COMMANDS.md) for the complete command reference including troubleshooting and metrics.

## Repo Layout

```
.
├── services/
│   ├── ingestor/          # tshark capture + MQTT publish (Python; host live, container pcap)
│   └── worker-postgres/   # MQTT subscribe → Postgres batch insert (Go; own go.mod, cmd/, internal/)
├── packages/
│   └── shared/            # pydantic config, MQTT topic constants (used by ingestor)
├── schema/
│   └── 001_init.sql       # hypertable DDL, indexes, retention (source of truth)
├── docker/
│   ├── mosquitto/         # MQTT broker compose + config
│   ├── postgres/          # TimescaleDB compose + init
│   ├── worker-postgres/   # Go worker compose (scratch image)
│   └── ingestor-pcap/     # pcap replay compose (replay profile)
├── tools/
│   └── parity/            # black-box e2e/parity harness (Go) used to validate the worker port
├── data/pcap/             # sample pcap for replay
├── docs/
│   └── PRD.md             # full product requirements document (18 sections)
├── Makefile               # orchestration: network/up/live/down/logs/e2e/test/lint/sync-schema
├── pyproject.toml         # Python workspace root: dev deps, ruff, mypy (ingestor + shared only)
└── README.md              # this file
```

## Full Spec

See [`docs/PRD.md`](docs/PRD.md) for the complete product requirements document, including:
- Detailed component specs
- Data model & DDL
- Failure semantics & error handling
- Testing strategy
- Security & observability
- Extensibility & future work

## License

None specified v1.
