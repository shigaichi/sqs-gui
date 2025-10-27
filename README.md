# SQS GUI

SQS GUI is a web application for exploring and managing Amazon SQS-compatible queues. It is designed for local development scenarios and ships with a simple Docker Compose stack that boots ElasticMQ and the GUI so you can inspect queues running on your machine. You can point the app at a real AWS account, but the server does not implement authentication or authorization, so it should never be exposed to the public internet.

## Features
- Queue inventory with name, type, creation time, message counts, encryption state, and deduplication flags
- Queue detail view showing tags, raw attributes, and quick actions to purge or delete queues
- Guided queue creation form with validation for FIFO and standard queues
- Interactive send/receive workspace that supports message attributes, FIFO group/deduplication fields, long polling, and delete operations

![Queues overview](docs/images/queues.png)

## Getting Started with Docker Compose
The easiest way to try the app locally is with the provided `compose.yaml`, which launches ElasticMQ (an SQS-compatible broker) alongside the pre-built GUI container.

```bash
# Start ElasticMQ and the GUI on ports 9324, 9325, and 8080
docker compose up -d

# Tail logs if you want to watch the services
docker compose logs -f
```

Once the containers are healthy, open http://localhost:8080/queues to browse queues and send messages. The ElasticMQ service automatically seeds the configuration from `elasticmq.conf`, and the GUI container inherits dummy credentials plus `AWS_SQS_ENDPOINT=http://elasticmq:9324` so everything works out of the box.

Stop the stack with `docker compose down` when you are done testing.

## Configuration and Environment Variables
The server relies on the standard AWS SDK configuration chain. Set the following variables (or configure your AWS profile/credentials file) before starting the app:

- `AWS_SQS_ENDPOINT` – Optional. HTTP endpoint for SQS-compatible services (e.g., `http://localhost:4566` for LocalStack or `http://elasticmq:9324` when using the compose stack).
- `AWS_REGION` – Optional. Defaults to `us-east-1` if not provided.
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` – Credentials for the target endpoint. For local stacks you can use dummy values.
