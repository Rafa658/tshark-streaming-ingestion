FROM python:3.13.14-slim

# Install Poetry and PostgreSQL client libraries
RUN apt-get update && apt-get install -y libpq-dev && rm -rf /var/lib/apt/lists/* && \
    pip install poetry

WORKDIR /app

COPY packages/shared/ /app/shared/
RUN cd /app/shared && poetry install

COPY services/worker-postgres/pyproject.toml /app/
RUN poetry install --only main --no-root

COPY services/worker-postgres/src /app/src
RUN poetry install

COPY services/worker-postgres/healthcheck.py /app/

CMD ["poetry", "run", "python", "-m", "worker_postgres"]