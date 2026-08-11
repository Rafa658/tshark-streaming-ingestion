# Commands to run in order

## 1. Setup environment
```bash
# Create virtual environment and install dependencies
python3 -m venv .venv
source .venv/bin/activate

# Install shared package
pip install -e packages/shared

# Install worker-postgres
pip install -e services/worker-postgres

# Install ingestor
pip install -e services/ingestor

# Install dev dependencies (for testing)
pip install pytest pytest-cov
```

## 2. Create Docker network
```bash
make network
```

## 3. Start core services
```bash
# Start Mosquitto, PostgreSQL, and Worker-Postgres
make up

# Wait for services to be healthy (check logs)
make logs
```

## 4. Run pcap replay demo
```bash
# Start pcap replay (uses sample.pcap)
make up-replay

# Monitor logs
docker compose -f docker/ingestor-pcap/docker-compose.yml --profile replay logs -f
```

## 5. Verify data in PostgreSQL
```bash
# Connect to PostgreSQL
psql -h localhost -U tshark_user -d tshark_db

# Query packet count
SELECT COUNT(*) FROM packets;

# Sample queries
SELECT ts, src_ip, dst_ip, proto FROM packets ORDER BY ts DESC LIMIT 10;
SELECT src_ip, dst_ip, COUNT(*) AS count FROM packets GROUP BY src_ip, dst_ip ORDER BY count DESC LIMIT 10;
```

## 6. Stop services
```bash
make down
```

## 7. Run tests
```bash
# Unit tests
make test

# Linting
make lint

# E2E test (with pcap replay)
make e2e
```

## 8. Live capture (macOS dev)
```bash
# In one terminal, start services
make up

# In another terminal, run live capture
source .venv/bin/activate
python -m ingestor --source live --interface en0

# Or with BPF filter
python -m ingestor --source live --interface en0 --bpf-filter "tcp port 443"
```

## 9. Check metrics
```bash
# Worker-Postgres metrics
curl http://localhost:8000/metrics

# Ingestor metrics (if running locally)
curl http://localhost:8000/metrics
```

## 10. Troubleshooting
```bash
# View all service logs
make logs

# Individual service logs
docker compose -f docker/mosquitto/docker-compose.yml logs -f
docker compose -f docker/postgres/docker-compose.yml logs -f
docker compose -f docker/worker-postgres/docker-compose.yml logs -f

# Restart a specific service
docker compose -f docker/worker-postgres/docker-compose.yml restart

# View buffer size and backpressure status
docker compose -f docker/worker-postgres/docker-compose.yml logs worker-postgres | grep -i "backpressure\|buffer"

# Check database connection
docker compose -f docker/postgres/docker-compose.yml exec postgres psql -U tshark_user -d tshark_db -c "SELECT 1;"
```