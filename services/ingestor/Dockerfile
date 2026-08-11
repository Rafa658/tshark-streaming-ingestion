FROM python:3.13.14-slim

# Install Poetry and tshark
RUN pip install poetry && \
    apt-get update && apt-get install -y tshark && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY packages/shared/ /app/shared/
RUN cd /app/shared && poetry install

COPY services/ingestor/pyproject.toml /app/
RUN poetry install --only main --no-root

COPY services/ingestor/src /app/src
RUN poetry install

CMD ["poetry", "run", "python", "-m", "ingestor"]