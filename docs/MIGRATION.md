# Poetry Workspace Conversion

## Changes Made

### Poetry Workspace Structure
- Root `pyproject.toml`: Poetry workspace configuration
- `packages/shared/pyproject.toml`: Shared package
- `services/worker-postgres/pyproject.toml`: Worker service
- `services/ingestor/pyproject.toml`: Ingestor service
- `poetry.lock`: Locked dependencies

### Updated Files
- `Makefile`: Poetry-based commands (`make install`, `make dev`, `make test`, `make lint`)
- `.gitignore`: Poetry-specific ignores, removed venv
- Dockerfiles: Poetry-based dependency installation
- Documentation: All docs updated for Poetry usage

### New Files
- `docs/POETRY.md`: Poetry workspace documentation
- `poetry.lock`: Placeholder for locked dependencies

### Key Commands
```bash
poetry install              # Install dependencies
poetry install --with dev   # Include dev dependencies
poetry run python -m ...    # Run modules
poetry run pytest           # Run tests
poetry run ruff check .     # Lint
poetry run mypy .           # Type check
```

## Migration Notes

- Removed pure venv usage
- All dependencies now managed via Poetry
- Docker builds use Poetry
- Test and lint commands use Poetry
- Documentation updated throughout

## Next Steps

1. Run `poetry install` to install dependencies
2. Run `poetry install --with dev` for dev dependencies
3. Run `poetry lock` to generate/ update lock file
4. Run `make test` to verify setup