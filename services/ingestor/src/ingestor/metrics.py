from prometheus_client import Counter, start_http_server

# Metrics
lines_read = Counter('ingestor_lines_read_total', 'Total lines read from tshark', ['service'])
msgs_published = Counter('ingestor_msgs_published_total', 'Total MQTT messages published', ['service'])
parse_errors = Counter('ingestor_parse_errors_total', 'Total parse errors', ['service'])


def start_metrics_server(port: int = 8000):
    start_http_server(port)


def record_line_read():
    lines_read.labels(service='ingestor').inc()


def record_msg_published():
    msgs_published.labels(service='ingestor').inc()


def record_parse_error():
    parse_errors.labels(service='ingestor').inc()