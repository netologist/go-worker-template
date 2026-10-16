# Go Worker Template

A production-grade, battle-tested Go background worker and asynchronous event processor template built with Hexagonal Architecture (ports & adapters), pluggable message listeners (PostgreSQL Outbox, RabbitMQ, Redpanda / Apache Kafka), concurrency worker pools, atomic idempotency deduplication, poison pill / dead letter queue (DLQ) routing, isolated health probes (`/livez`, `/readyz`), hermetic toolchains (Mise), and a zero-trust CI/CD pipeline.

## Architecture & Worker Paradigms

| Dimension | Specification |
| --- | --- |
| **Entrypoint** | `cmd/worker/main.go` (Worker Pool / Consumer Loop / Graceful Drain) |
| **Inbound Adapters** | Pluggable `MessageListener` (PostgreSQL Outbox `SKIP LOCKED`, RabbitMQ AMQP 0-9-1, Redpanda / Apache Kafka) |
| **Execution Engine** | `golang.org/x/sync/errgroup` with configurable concurrency (`WORKER_CONCURRENCY`) |
| **Deduplication** | Atomic `IdempotencyStore` (`TryAcquire`, `MarkCompleted`, `MarkFailed`) |
| **Error Classification** | `ErrTransient` (NACK / Exponential Retry) vs `ErrFatal` / Poison Pill (DLQ Routing) |
| **Health & Probes** | Lightweight HTTP server (`/livez`, `/readyz`, Port `8081`) |
| **Graceful Shutdown** | Consumer Pause $\to$ Worker Goroutines Drain $\to$ Final Ack $\to$ DB Close |

## Documentation

- [Project Folder Structure](docs/folder-structure.md) - Breakdown of hexagonal architecture layers, worker consumers, and directory layout.
- [Technology Stack & Tools](docs/tech-stack.md) - Rationale, ecosystem tools, runtime libraries, and operational stack.
- [CI/CD & Security Pipeline](docs/pipeline.md) - Zero-trust GitHub Actions pipeline, secret scanning, and vulnerability gates.
- [Transformation Guide](docs/reviews/2026-10-10-go-worker-template-transformation-guide.md) - Architectural review and transition guide from service to worker.
- [Template Usage & Adoption Guide](docs/template-guide.md) - How to instantiate and tailor this repository as a production worker.


## Creating a New Worker from this Template

You can instantiate a new worker service from this template using either the official Go `gonew` tool or GitHub's **Use this template** button. Because `gonew` only replaces Go source imports, use `mise run template:init` to ensure all configurations (YAML, TOML, CI workflows, and code generators) are synchronized.

### Option A: Using `gonew` + `mise run template:init` (CLI Workflow)

```bash
# 1. Install gonew (official Go project templating tool)
go install golang.org/x/tools/cmd/gonew@latest

# 2. Instantiate a new worker directory from this template
gonew github.com/netologist/go-worker-template github.com/your-org/your-worker

# 3. Enter the new project directory
cd your-worker

# 4. Install toolchain via mise
mise install

# 5. Run template:init to sync non-Go configurations (.golangci.yml, .mockery.yaml, CI, mise.toml)
mise run template:init github.com/your-org/your-worker
```

### Option B: Using GitHub Web + `mise run template:init`

```bash
# 1. Click "Use this template" on GitHub and create your new repo
git clone https://github.com/your-org/your-worker.git
cd your-worker

# 2. Install toolchain via mise
mise install

# 3. Run template:init to replace all module paths, regenerate code, and verify tests
mise run template:init github.com/your-org/your-worker
```

For a step-by-step breakdown on replacing the reference "Task" domain with your own business domain logic, refer to the [Template Adoption Guide](docs/template-guide.md).
## Prerequisites

- [Mise](https://mise.jdx.dev/) (manages Go runtime and toolchains)
- [Docker](https://docs.docker.com/get-docker/) & Docker Compose (for PostgreSQL, RabbitMQ, and Redpanda)

## Quickstart

```bash
# 1. Install toolchain via mise
mise install

# 2. Start PostgreSQL, RabbitMQ, and Redpanda dependencies
mise run docker:compose-up

# 3. Apply database migrations (tasks & processed_messages idempotency table)
mise run migrate:up

# 4. Start worker process with live-reload (Air)
mise run dev
```

### Broker Selection

The worker supports hot-swapping inbound broker consumers via environment variable or CLI flag:

```bash
# PostgreSQL Outbox Poller (Default)
BROKER_KIND=postgres_outbox mise run dev

# RabbitMQ AMQP 0-9-1 Consumer
BROKER_KIND=rabbitmq mise run dev

# Redpanda / Apache Kafka Stream Consumer
BROKER_KIND=redpanda mise run dev
# or:
BROKER_KIND=kafka KAFKA_BROKERS=localhost:9092 KAFKA_TOPICS=tasks mise run dev

# Or via binary CLI flags:
./worker --broker-kind=redpanda --concurrency=20
```

### Probe Endpoints

The worker exposes isolated HTTP health probes on port `8081`:
- `GET http://localhost:8081/livez` - Process liveness (200 OK)
- `GET http://localhost:8081/readyz` - Dependency readiness check (PostgreSQL ping)

## Common Mise Commands

```bash
mise run dev              # Run worker with Air live-reload
mise run test             # Run unit, usecase, and architectural boundary tests
mise run test:coverage    # Run tests with race detector and show coverage
mise run lint             # Run golangci-lint
mise run format           # Format Go code with goimports
mise run secret:check     # Scan for secrets with Gitleaks
mise run vuln             # Scan for vulnerabilities with govulncheck
mise run generate         # Regenerate SQL queries and mocks
mise run docker:build     # Build scratch production container
```
