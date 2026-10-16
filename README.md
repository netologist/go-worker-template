# Go Worker Template

A production-grade, battle-tested Go background worker and asynchronous event processor template built with Hexagonal Architecture (ports & adapters), pluggable message listeners (PostgreSQL Outbox, RabbitMQ), concurrency worker pools, atomic idempotency deduplication, poison pill / dead letter queue (DLQ) routing, isolated health probes (`/livez`, `/readyz`), hermetic toolchains (Mise), and a zero-trust CI/CD pipeline.

## Architecture & Worker Paradigms

| Dimension | Specification |
| --- | --- |
| **Entrypoint** | `cmd/worker/main.go` (Worker Pool / Consumer Loop / Graceful Drain) |
| **Inbound Adapters** | Pluggable `MessageListener` (PostgreSQL Outbox `SKIP LOCKED`, RabbitMQ AMQP 0-9-1) |
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

## Prerequisites

- [Mise](https://mise.jdx.dev/) (manages Go runtime and toolchains)
- [Docker](https://docs.docker.com/get-docker/) & Docker Compose (for PostgreSQL and RabbitMQ)

## Quickstart

```bash
# 1. Install toolchain via mise
mise install

# 2. Start PostgreSQL and RabbitMQ dependencies
mise run docker:compose-up

# 3. Apply database migrations (tasks & processed_messages idempotency table)
mise run migrate:up

# 4. Start worker process with live-reload (Air)
mise run dev
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
