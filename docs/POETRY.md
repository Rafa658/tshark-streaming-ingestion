# Poetry Workspace

This project uses Poetry for dependency management and packaging. It's configured as a workspace monorepo with the following structure:

```
tshark-streaming-ingestion/
├── packages/shared/          # shared utilities
├── services/worker-postgres/ # MQTT → PostgreSQL worker
├── services/ingestor/        # tshark → MQTT ingestor
└── pyproject.toml            # workspace root
```

## Installation

```bash
# Install Poetry (if not installed)
curl -sSL https://install.python-poetry.org | python3 -

# Install all dependencies
make install

# Install dev dependencies (for testing/linting)
make dev
```

## Common Commands

```bash
poetry install              # Install dependencies
poetry install --with dev   # Include dev dependencies
poetry lock                 # Update lock file
poetry run python -m ...    # Run Python modules
poetry run pytest           # Run tests
poetry run ruff check .     # Lint
poetry run mypy .           # Type check
```

## Workspace Structure

- **Root pyproject.toml**: Defines workspace members and dev dependencies
- **packages/shared**: Shared configuration, models, and utilities
- **services/**: Individual services with their own dependencies
- **poetry.lock**: Locked dependency versions (committed to repo)

## Dependency Management

Add a new dependency:
```bash
# To a service
cd services/worker-postgres
poetry add paho-mqtt

# To the workspace root (dev dependency)
poetry add pytest --group dev
```

Update all dependencies:
```bash
poetry update
```