# Commands to get Tshark Streaming Ingestion working

## Prerequisites
- Docker & Docker Compose running
- Poetry (install via `curl -sSL https://install.python-poetry.org | python3 -`)
- Python 3.13.14
- macOS: `tshark` via `brew install tshark`

## Setup (one-time)

```bash
# Clone repo
cd tshark-streaming-ingestion

# Install all packages with Poetry
make install

# Install dev dependencies (for testing/linting)
make dev

# Fetch sample pcap (if needed)
make fetch-sample
```

## Start the pipeline

```bash
# Create Docker network
make network

# Start Mosquitto, PostgreSQL, Worker-Postgres
make up

# Wait ~10 seconds for services to be healthy
make logs

# Run pcap replay (uses data/pcap/sample.pcap)
make up-replay

# Monitor logs
docker compose -f docker/ingestor-pcap/docker-compose.yml --profile replay logs -f
```

## Verify data in PostgreSQL

```bash
# Connect to database
psql -h localhost -U tshark_user -d tshark_db -c "SELECT COUNT(*) FROM packets;"

# Sample queries
psql -h localhost -U tshark_user -d tshark_db -c "SELECT ts, src_ip, dst_ip, proto FROM packets ORDER BY ts DESC LIMIT 10;"
psql -h localhost -U tshark_user -d tshark_db -c "SELECT src_ip, dst_ip, COUNT(*) AS cnt FROM packets GROUP BY src_ip, dst_ip ORDER BY cnt DESC LIMIT 10;"
```

## Live capture (macOS dev, tshark on host)

```bash
# Terminal 1: Start services
make up

# Terminal 2: Run live capture
poetry run python -m ingestor --source live --interface en0

# Optional: With BPF filter
poetry run python -m ingestor --source live --interface en0 --bpf-filter "tcp port 443"
```

## Check metrics

```bash
# Worker-Postgres metrics
curl http://localhost:8000/metrics

# Ingestor metrics (if running locally)
curl http://localhost:8000/metrics
```

## Stop the pipeline

```bash
make down
```

## Tests

```bash
# Unit tests (13 tests pass)
make test

# Linting
make lint

# E2E with pcap replay
make e2e
```

## Troubleshooting

```bash
# All logs
make logs

# Individual service logs
docker compose -f docker/mosquitto/docker-compose.yml logs -f
docker compose -f docker/postgres/docker-compose.yml logs -f
docker compose -f docker/worker-postgres/docker-compose.yml logs -f

# Check database connection
docker compose -f docker/postgres/docker-compose.yml exec postgres psql -U tshark_user -d tshark_db -c "SELECT 1;"

# Restart specific service
docker compose -f docker/worker-postgres/docker-compose.yml restart

# Check backpressure/buffer status
docker compose -f docker/worker-postgres/docker-compose.yml logs worker-postgres | grep -i "backpressure\|buffer"

# Poetry dependency management
poetry lock                   # Update lock file
poetry install                # Install dependencies
poetry install --with dev     # Install with dev dependencies
poetry update                 # Update dependencies
```

## File sync (after schema changes)

```bash
make sync-schema
```