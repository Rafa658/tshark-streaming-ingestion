.PHONY: network up up-replay live down logs e2e test lint sync-schema fetch-sample install dev observability obs-down help

help:
	@echo "Available targets:"
	@echo "  make install       - Install all packages with Poetry"
	@echo "  make dev           - Install dev dependencies with Poetry"
	@echo "  make network       - Create external Docker network 'ingestion-net'"
	@echo "  make up            - Start mosquitto, postgres, worker-postgres"
	@echo "  make up-replay     - Start all services + ingestor-pcap (replay profile)"
	@echo "  make live          - Live capture on host interface (sudo, IFACE=en0)"
	@echo "  make down          - Stop all services"
	@echo "  make logs          - Show logs from all services"
	@echo "  make e2e           - Run end-to-end test with pcap replay"
	@echo "  make test          - Run unit + integration tests"
	@echo "  make lint          - Run ruff + mypy"
	@echo "  make sync-schema       - Copy schema/001_init.sql to docker/postgres/postgres-init/"
	@echo "  make fetch-sample      - Download sample pcap file to data/pcap/sample.pcap"
	@echo "  make observability     - Start Prometheus + Grafana dashboard"
	@echo "  make obs-down          - Stop Prometheus + Grafana"
	@echo ""
	@echo "Note: Requires docker-compose. Install with: pip install docker-compose"

install:
	cd packages/shared && poetry install --no-root
	cd ../ingestor && poetry install --no-root
	cd ../.. && poetry install --no-root

dev:
	poetry install --with dev --no-root

network:
	docker network create ingestion-net 2>/dev/null || true

up: network
	docker compose -f docker/mosquitto/docker-compose.yml up -d
	docker compose -f docker/postgres/docker-compose.yml up -d
	docker compose -f docker/worker-postgres/docker-compose.yml up -d
	docker compose -f docker/prometheus/docker-compose.yml up -d
	docker compose -f docker/grafana/docker-compose.yml up -d

up-replay: up
	docker compose -f docker/ingestor-pcap/docker-compose.yml --profile replay up -d

live: up
	@echo "Live capture on $${IFACE:-en0} (sudo needed for tshark BPF access)..."
	@echo "Override: make live IFACE=en1  |  make live LIVE_ARGS='--bpf-filter \"not port 22\"'"
	sudo env \
		MQTT_HOST=localhost \
		METRICS_PORT=$${INGESTOR_METRICS_PORT:-8002} \
		PYTHONPATH=packages/shared/src:services/ingestor/src \
		$$(which poetry) run python -m ingestor \
		--source live --interface $${IFACE:-en0} $${LIVE_ARGS}

down:
	docker compose -f docker/mosquitto/docker-compose.yml down
	docker compose -f docker/postgres/docker-compose.yml down
	docker compose -f docker/worker-postgres/docker-compose.yml down
	docker compose -f docker/ingestor-pcap/docker-compose.yml down
	docker compose -f docker/prometheus/docker-compose.yml down
	docker compose -f docker/grafana/docker-compose.yml down

logs:
	docker compose -f docker/mosquitto/docker-compose.yml logs -f &
	docker compose -f docker/postgres/docker-compose.yml logs -f &
	docker compose -f docker/worker-postgres/docker-compose.yml logs -f

e2e: sync-schema up-replay
	@echo "Waiting for services to start..."
	sleep 5
	@echo "Running pcap replay e2e test..."
	@echo "TODO: implement e2e test"
	docker compose -f docker/ingestor-pcap/docker-compose.yml --profile replay logs -f
	make down

test:
	poetry run pytest

lint:
	poetry run ruff check .
	poetry run mypy .

sync-schema:
	cp schema/001_init.sql docker/postgres/postgres-init/001-init.sql

fetch-sample:
	curl -o data/pcap/sample.pcap https://gitlab.com/wireshark/wireshark/-/raw/master/test/captures/http.cap

observability: network
	docker compose -f docker/cadvisor/docker-compose.yml up -d
	docker compose -f docker/prometheus/docker-compose.yml up -d
	docker compose -f docker/grafana/docker-compose.yml up -d

obs-down:
	docker compose -f docker/grafana/docker-compose.yml down
	docker compose -f docker/prometheus/docker-compose.yml down
	docker compose -f docker/cadvisor/docker-compose.yml down