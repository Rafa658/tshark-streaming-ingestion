# Tshark Streaming Ingestion

Streaming pipeline that ingests tshark packet capture data via MQTT into a PostgreSQL instance with TimescaleDB (hypertable).

## Architecture

```
tshark (host) ──► Mosquitto MQTT broker (no auth v1)
                      │
                      ├─► worker-postgres ──► PostgreSQL + TimescaleDB (hypertable)
                      │
                      └─► (future: worker-s3, etc. each subscribe to same topic)
```

### Key Architecture Choices

- **Pure MQTT fan-out** — adding S3 subscriber = another MQTT client, no code change to ingestor
- **Raw-data-first** — store full tshark `-T ek` line as JSONB (`payload` column), transform later
- **Host-side ingestor for live capture** (macOS dev), containerized pcap replay; Linux prod swap path documented
- **Single-thread worker** — paho-mqtt + psycopg v3, multi-row INSERT, batch @500 rows or 1s
- **Backpressure on DB failures** — stop acking MQTT, buffer capped at 50k rows
- **Split-compose** — one service per docker-compose folder, shared external network, root Makefile orchestration
- **At-least-once delivery** — MQTT QoS1, accept duplicates on retry (no dedup v1)
- **Observability** — loguru JSON logging, `/metrics` exposed per service (no prom/grafana v1)

## Quickstart

### Prerequisites

- macOS dev: `tshark` installed via Homebrew (`brew install tshark`)
- Docker & Docker Compose
- Poetry (`curl -sSL https://install.python-poetry.org | python3 -`)
- Python 3.13.14

### Setup

```bash
# Clone and navigate
git clone <repo>
cd tshark-streaming-ingestion

# Install all packages with Poetry
make install

# Install dev dependencies (for testing/linting)
make dev

# Create shared Docker network
make network

# Start the stack
make up

# In another terminal, run live capture via Poetry
poetry run python -m ingestor --source live --interface en0

# Or run pcap replay demo
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
```

### Run Tests

```bash
make test          # unit + integration (Poetry pytest)
make lint          # ruff + mypy (Poetry)
make e2e           # full end-to-end with pcap replay
```

### Teardown

```bash
make down          # stop all services
```

## Poetry Workspace

This is a Poetry workspace monorepo with the following structure:
- `packages/shared` — shared utilities and configuration
- `services/worker-postgres` — MQTT to PostgreSQL worker
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

## Commands

See [`COMMANDS.md`](COMMANDS.md) for the complete command reference including troubleshooting and metrics.

## Repo Layout

```
.
├── services/
│   ├── ingestor/          # tshark capture + MQTT publish (host live, container pcap)
│   └── worker-postgres/   # MQTT subscribe → Postgres batch insert
├── packages/
│   └── shared/            # pydantic config, MQTT topic constants, shared models
├── schema/
│   └── 001_init.sql       # hypertable DDL, indexes, retention (source of truth)
├── docker/
│   ├── mosquitto/         # MQTT broker compose + config
│   ├── postgres/          # TimescaleDB compose + init
│   ├── worker-postgres/   # worker compose
│   └── ingestor-pcap/     # pcap replay compose (replay profile)
├── data/pcap/             # sample pcap for replay
├── docs/
│   └── PRD.md             # full product requirements document (18 sections)
├── Makefile               # orchestration: network/up/down/logs/e2e/test/lint/sync-schema
├── pyproject.toml         # workspace root: dev deps, ruff, mypy
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
