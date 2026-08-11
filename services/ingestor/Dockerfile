FROM python:3.13.14-slim

# Install Poetry and tshark
RUN pip install poetry && \
    apt-get update && apt-get install -y tshark && rm -rf /var/lib/apt/lists/*

# The repo layout is preserved inside the image so the relative path dependency
# on ../../packages/shared resolves the same way it does on a developer machine.
WORKDIR /app/services/ingestor

COPY packages/shared/ /app/packages/shared/
COPY services/ingestor/pyproject.toml /app/services/ingestor/

RUN poetry install --only main --no-root

COPY services/ingestor/src /app/services/ingestor/src
RUN poetry install --only main

CMD ["poetry", "run", "python", "-m", "ingestor"]
