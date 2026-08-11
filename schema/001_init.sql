CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE packets (
    ts TIMESTAMPTZ NOT NULL,
    src_ip INET NOT NULL,
    dst_ip INET NOT NULL,
    src_port INTEGER,
    dst_port INTEGER,
    proto TEXT NOT NULL,
    length INTEGER NOT NULL,
    payload JSONB NOT NULL
);

SELECT create_hypertable('packets', 'ts', chunk_time_interval => INTERVAL '1 hour');

CREATE INDEX idx_packets_talker ON packets (src_ip, dst_ip, ts);
CREATE INDEX idx_packets_payload ON packets USING GIN (payload jsonb_path_ops);

SELECT add_retention_policy('packets', INTERVAL '1 day');