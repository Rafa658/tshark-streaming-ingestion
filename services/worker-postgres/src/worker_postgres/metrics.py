from prometheus_client import Counter, Gauge, start_http_server

# Metrics
msgs_consumed = Counter('worker_msgs_consumed_total', 'Total MQTT messages consumed', ['service'])
rows_inserted = Counter('worker_rows_inserted_total', 'Total rows inserted into DB', ['service'])
batch_flushes = Counter('worker_batch_flush_total', 'Total batch flushes', ['service'])
dead_letters = Counter('worker_deadletter_total', 'Total dead-letter messages', ['service'])
db_errors = Counter('worker_db_errors_total', 'Total DB errors', ['service'])
buffer_size = Gauge('worker_buffer_size', 'Current buffer size', ['service'])


def start_metrics_server(port: int = 8000):
    start_http_server(port)


def record_msg_consumed():
    msgs_consumed.labels(service='worker-postgres').inc()


def record_rows_inserted(count: int):
    rows_inserted.labels(service='worker-postgres').inc(count)


def record_batch_flush():
    batch_flushes.labels(service='worker-postgres').inc()


def record_dead_letter():
    dead_letters.labels(service='worker-postgres').inc()


def record_db_error():
    db_errors.labels(service='worker-postgres').inc()


def set_buffer_size(size: int):
    buffer_size.labels(service='worker-postgres').set(size)