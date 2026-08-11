FROM python:3.13.14-slim

# Install Poetry and PostgreSQL client libraries
RUN apt-get update && apt-get install -y libpq-dev && rm -rf /var/lib/apt/lists/* && \
    pip install poetry

# The repo layout is preserved inside the image so the relative path dependency
# on ../../packages/shared resolves the same way it does on a developer machine.
WORKDIR /app/services/worker-postgres

COPY packages/shared/ /app/packages/shared/
COPY services/worker-postgres/pyproject.toml /app/services/worker-postgres/

RUN poetry install --only main --no-root

COPY services/worker-postgres/src /app/services/worker-postgres/src
COPY services/worker-postgres/healthcheck.py /app/services/worker-postgres/
RUN poetry install --only main

CMD ["poetry", "run", "python", "-m", "worker_postgres"]
