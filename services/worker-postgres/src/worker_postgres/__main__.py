from tshark_shared.logging import logger
from worker_postgres.consumer import WorkerConsumer


if __name__ == "__main__":
    logger.info("Starting worker-postgres...")
    worker = WorkerConsumer()
    worker.start()