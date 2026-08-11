# Setup and Commands for Tshark Streaming Ingestion (Poetry)

## Commands to run in order

### 1. Initial setup with Poetry
```bash
# Install Poetry if not installed
curl -sSL https://install.python-poetry.org | python3 -

# Install all packages
make install

# Install dev dependencies (for testing/linting)
make dev
```

### 2. Start the pipeline
```bash
make network
make up
make up-replay
make logs
```

### 3. Verify data
```bash
psql -h localhost -U tshark_user -d tshark_db -c "SELECT COUNT(*) FROM packets;"
```

### 4. Stop
```bash
make down
```

### 5. Tests
```bash
make test
make lint
```

### 6. Live capture (macOS)
```bash
make up
poetry run python -m ingestor --source live --interface en0
```

## Full reference: COMMANDS.md

## Poetry workspace
```bash
poetry install              # Install dependencies
poetry install --with dev   # Install with dev deps
poetry lock                 # Update lock file
poetry run python -m ingestor --source live --interface en0
```