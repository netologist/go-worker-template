# Developer User Guide

This guide provides end-to-end instructions for developing, operating, testing, debugging, and deploying the **Go Worker Template** background worker architecture (Hexagonal Architecture, pluggable message listeners, PostgreSQL persistence with UUIDv7, hermetic `mise` toolchains, and zero-trust CI/CD standards).

---

## 1. Architecture & Core Capabilities

The worker follows strict architectural boundaries designed for high maintainability, testability, and operational resilience:

- **Hexagonal Architecture (Ports & Adapters):** Core business logic (`internal/domain`) and application use cases (`internal/usecase`) are completely decoupled from external message brokers and database drivers. Message listeners (`internal/adapter/consumer`), probe servers (`internal/adapter/probe`), and PostgreSQL storage (`internal/adapter/postgres`) operate as pluggable adapters.
- **Pluggable Message Listeners:** Unified `MessageListener` interface supporting PostgreSQL Transactional Outbox (SKIP LOCKED), RabbitMQ (AMQP 0-9-1), and Redpanda / Apache Kafka (`franz-go`) with automatic panic recovery.
- **Atomic Idempotency Deduplication:** Outbound `IdempotencyStore` guarantees exact-once execution semantics across distributed consumers.
- **Poison Pill & DLQ Routing:** Differentiates transient errors (`ErrTransient` $\to$ NACK / exponential retry) from fatal errors / poison pills (`ErrFatal` $\to$ Dead Letter Queue).
- **Type-Safe Persistence (ADR-0003):** Compile-time SQL query validation with `sqlc`, version-controlled migrations with `goose`, high-performance connection pooling with `pgx/v5`, and native B-Tree-friendly `UUIDv7` primary keys.
- **Hermetic Toolchain (ADR-0001):** All development CLI tools (`air`, `goose`, `sqlc`, `mockery`, `golangci-lint`, `gotestsum`, `gitleaks`, `govulncheck`, `lefthook`) are pinned directly inside `go.mod` via Go's native `tool` directive and executed hermetically through `mise`.
- **Dual-Loop Local Development (ADR-0006):** Decoupled workflows provide fast live-reloading via `Air` (`mise run dev`) and interactive step debugging via native `Delve` inside VS Code (`F5`).
---

## 2. Prerequisites & Environment Setup

To run and develop this project, you only need two tools installed on your host machine:

| Tool | Minimum Version | Responsibility |
| :--- | :--- | :--- |
| **[Mise](https://mise.jdx.dev/)** | Latest | Manages Go runtime (`1.27.2`) and task orchestration. |
| **[Docker & Docker Compose](https://docs.docker.com/get-docker/)** | v2.20+ | Runs local PostgreSQL database and containerized suites. |

> **Note:** You do **not** need to install Go CLI utilities globally (`go install ...`). Every tool is version-pinned in `go.mod` (callable via `go tool <tool>`) and CLI tools (`buf`, `grpcurl`) are managed directly through `mise`.

### Bootstrap Local Environment

```bash
# 1. Clone repository and navigate to the project root
git clone https://github.com/netologist/go-worker-template.git
cd go-worker-template

# 2. Install Go runtime and hermetic toolchain via mise
mise install

# 3. Install Git pre-commit and pre-push hooks (managed by Lefthook)
mise run hooks:install
```

---

## 3. Quickstart

Get the complete service running locally with a PostgreSQL database in 4 steps:

```bash
# Step 1: Create local environment configuration from template
cp .env.example .env

# Step 2: Start PostgreSQL, RabbitMQ, and Redpanda containers
mise run docker:compose-up

# Step 3: Apply database schema migrations
mise run migrate:up

# Step 4: Run worker process
go run ./cmd/worker
```

The worker starts consuming tasks and exposes lightweight probes on `http://localhost:8081`.

### Smoke Verification

In a new terminal window, verify the health and readiness probes:

```bash
# 1. Liveness check
curl -i http://localhost:8081/livez

# Expected Response: HTTP/1.1 200 OK
# {"status":"alive"}

# 2. Readiness check
curl -i http://localhost:8081/readyz

# Expected Response: HTTP/1.1 200 OK
# {"status":"ready"}
```

---

## 4. Configuration & Environment Variables

Configuration is declared in `internal/config/config.go` and parsed from environment variables using `caarlos0/env/v11`. All settings are strictly validated on application startup via `go-playground/validator` (fail-fast principle).

Below is the complete reference of environment variables defined in `.env.example`:

| Environment Variable | Type / Format | Default | Description & Constraints |
| :--- | :--- | :--- | :--- |
| `APP_ENV` | `string` | `development` | Target environment (`development`, `staging`, `production`, `test`). |
| `LOG_LEVEL` | `string` | `info` | Structured logging level (`debug`, `info`, `warn`, `error`). |
| `BROKER_KIND` | `string` | `postgres_outbox` | Inbound consumer type (`postgres_outbox`, `rabbitmq`, `redpanda`, `kafka`). |
| `CONSUMER_GROUP` | `string` | `default-worker-group` | Consumer group identifier for message queues. |
| `WORKER_CONCURRENCY` | `int` | `10` | Number of concurrent worker goroutines. |
| `BATCH_SIZE` | `int` | `10` | Maximum messages pulled per batch. |
| `POLL_INTERVAL` | `duration` | `1s` | Polling frequency for outbox worker. |
| `PROCESSING_TIMEOUT` | `duration` | `30s` | Maximum execution time per task before timeout context cancellation. |
| `MAX_RETRIES` | `int` | `3` | Maximum retry attempts for transient errors before DLQ routing. |
| `DLQ_QUEUE_NAME` | `string` | `tasks_dlq` | Destination dead letter queue or topic name. |
| `IDEMPOTENCY_TTL` | `duration` | `168h` | Time-to-live for processed message deduplication records. |
| `RABBITMQ_URL` | `string` | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection URI. |
| `RABBITMQ_QUEUE` | `string` | `tasks_queue` | Target RabbitMQ queue. |
| `RABBITMQ_PREFETCH_COUNT` | `int` | `20` | RabbitMQ channel prefetch count. |
| `KAFKA_BROKERS` | `string` | `localhost:9092` | Kafka / Redpanda broker endpoints. |
| `KAFKA_TOPICS` | `string` | `tasks` | Target Kafka topic(s). |
| `PROBE_PORT` | `int` | `8081` | Port for the health and readiness HTTP server (`1`–`65535`). |
| `SHUTDOWN_TIMEOUT` | `duration` | `30s` | Maximum time allowed to drain in-flight worker goroutines during shutdown. |
| `DATABASE_URL` | `string` | *(Required)* | PostgreSQL connection URI. |
| `DB_MAX_CONNS` | `int32` | `25` | Maximum active database connections in `pgxpool`. |
| `DB_MIN_CONNS` | `int32` | `2` | Minimum idle database connections maintained in `pgxpool`. |
| `DB_MAX_CONN_LIFETIME` | `duration` | `30m` | Maximum lifetime of a database connection. |
| `DB_MAX_CONN_IDLE_TIME` | `duration` | `5m` | Maximum idle time before closing an unused connection. |
| `DB_HEALTH_CHECK_PERIOD`| `duration` | `1m` | Frequency of background pool health checks. |
| `DB_PING_TIMEOUT` | `duration` | `5s` | Timeout for pinging PostgreSQL during startup and health check probes. |

---

## 5. Development Workflows & Debugging

As documented in [ADR-0006](decisions/0006-local-development-air-delve.md), two distinct workflows are supported:

```mermaid
graph TD
    subgraph "Fast Inner Loop"
        AirTask["mise run dev"] --> PostgresCheck["Verify PostgreSQL Container"]
        PostgresCheck --> MigrateCheck["Apply Pending Migrations"]
        MigrateCheck --> AirWatch["Air File Watcher"]
        AirWatch -->|"File changes (*.go)"| Rebuild["Recompile & Graceful Restart via SIGINT"]
    end

    subgraph "Interactive Debugging Loop"
        VSCode["VS Code / Cursor (F5)"] --> LaunchJSON[".vscode/launch.json"]
        LaunchJSON --> Delve["Delve (dlv debugger)"]
        Delve --> Breakpoints["Breakpoints, Variable Inspection & Call Stacks"]
    end
```

### 1. Air Live-Reloading (`mise run dev`)

For rapid feature development with automatic recompilation on file change:

```bash
mise run dev
```

This single command automatically:
1. Ensures the PostgreSQL container is running (`docker compose up -d --wait`).
2. Applies any pending database migrations (`go tool goose up`).
3. Starts `Air` (`go tool air`) to monitor `.go`, `.yaml`, and `.html` files, gracefully restarting the process upon file modifications.

### 2. VS Code & Native Delve Debugging

For step-through debugging with breakpoints, call stack inspection, and variable inspection:

1. If `mise run dev` is running, **stop it** to avoid port `:8081` collisions.
2. Ensure dependencies are running: `mise run docker:compose-up`.
3. Open VS Code and switch to the **Run and Debug** view (`Ctrl+Shift+D` / `Cmd+Shift+D`).
4. Select **Debug Worker (cmd/worker)** and press **F5**.
5. Set breakpoints in consumers, use cases, or domain files.

To debug a single test package, place your cursor in the target test file, select **Debug Current Test** in VS Code, and press **F5**.

---

## 6. Code Generation & Persistence Workflow

All SQL queries and test mocks are generated from authoritative source files:

```mermaid
graph LR
    SQL["database/queries/*.sql"] -->|mise run generate:sqlc| SQLCGen["internal/gen/sqlc/"]
    Interfaces["internal/domain/ports.go"] -->|mise run generate:mocks| MocksGen["internal/gen/mocks/domain/"]
```

### Generator Commands

| Command | Description |
| :--- | :--- |
| `mise run generate` | Executes all code generators in dependency order (SQLC, Mockery). |
| `mise run generate:sqlc` | Compiles SQL queries in `database/queries/*.sql` into type-safe Go persistence methods. |
| `mise run generate:mocks` | Generates interface test mocks into `internal/gen/mocks/domain/` via Mockery. |
| `mise run mocks:check` | Verifies in CI that generated mocks are up to date without unstaged drift. |

### Adding a New Task or Query

1. Define task domain logic, errors, and invariants in `internal/domain/`.
2. Add required queries in `database/queries/` and run `mise run generate:sqlc`.
3. Update use cases in `internal/usecase/process_task.go`.
4. Run `mise run generate:mocks && mise run lint && mise run test` to verify.

---

## 7. Database Operations & Migrations

Database operations are managed via `goose` and `sqlc`.

### Managing PostgreSQL Containers

```bash
# Start PostgreSQL container and wait for healthy status
mise run docker:compose-up

# Stop PostgreSQL container and wipe persistent volume data
mise run docker:compose-down

# Open interactive PostgreSQL shell (psql)
mise run psql
```

### Schema Migrations (Goose)

Migration files are stored in `database/migrations/` with timestamp prefixes:

```bash
# 1. Create a new migration file
go tool goose -dir database/migrations create add_category_to_items sql

# 2. Apply all pending migrations
mise run migrate:up

# 3. Rollback the most recent migration
mise run migrate:down

# 4. Check migration status
mise run migrate:status
```

### Primary Key Design: UUIDv7

All database tables use **UUIDv7** primary keys:
- **Time-Ordered:** Preserves B-Tree index locality, avoiding random page insertion fragmentation typical of UUIDv4.
- **Decentralized:** Primary keys are generated client-side or in domain logic without roundtrips to database sequences.
- **PostgreSQL 16 Native:** Generated using PostgreSQL native functions or standard Go UUID libraries (`github.com/google/uuid`).

---

## 8. Health Probes & Monitoring

The worker exposes isolated HTTP health check endpoints on port `:8081`:

### Liveness Probe (`GET /livez`)
Returns `200 OK` as long as the worker process is running:
```bash
curl -i http://localhost:8081/livez
```

### Readiness Probe (`GET /readyz`)
Returns `200 OK` when the database connection pool is healthy and reachable:
```bash
curl -i http://localhost:8081/readyz
```
If database connectivity is lost or during graceful shutdown draining, `/readyz` returns `503 Service Unavailable`.

---

## 9. Inbound Consumers & Asynchronous Processing

### Consumer Modes (`BROKER_KIND`)

1. **PostgreSQL Outbox (`postgres_outbox`)**:
   - Polls unprocessed messages using `FOR UPDATE SKIP LOCKED`.
   - Guarantees transactional consistency with business data.

2. **RabbitMQ (`rabbitmq`)**:
   - Connects via AMQP 0-9-1 using `RABBITMQ_URL` and `RABBITMQ_QUEUE`.
   - Uses explicit manual ACKs on completion and NACKs on transient failure.

3. **Redpanda / Apache Kafka (`kafka` / `redpanda`)**:
   - Uses pure Go `franz-go` consumer group reading from `KAFKA_TOPICS`.
   - Commits partition offsets upon successful task processing.

### Idempotency & Error Handling

- **Deduplication**: `ProcessTaskUseCase` invokes `IdempotencyStore.TryAcquire(ctx, messageID)`. If already processed, the message is safely acknowledged without duplicate side effects.
- **Transient Errors**: If a task fails with `domain.ErrTransient`, the consumer retries with backoff up to `MAX_RETRIES`.
- **Poison Pills**: Malformed or unrecoverable tasks (`domain.ErrFatal`) are routed to `DeadLetterPublisher` and acknowledged to prevent head-of-line blocking.

## 10. Testing & Quality Assurance

The repository includes a comprehensive testing pyramid:

```bash
# 1. Run unit, client, and architectural tests with race detector
mise run test

# 2. Verify Hexagonal boundary rules (architecture tests)
mise run test:arch

# 3. Compute code coverage summary
mise run test:coverage

# 4. Open interactive HTML coverage report
mise run test:coverage:html

# 5. Run mutation testing on domain layer (Gremlins)
mise run test:mutation

# 6. Run end-to-end suite with Ginkgo/Gomega against real PostgreSQL
mise run test:e2e
```

### Static Analysis & Security Scanning

```bash
# Run full golangci-lint analysis
mise run lint

# Auto-fix linting issues
mise run lint:fix

# Format imports according to module conventions
mise run format

# Scan codebase for committed secrets (Gitleaks)
mise run secret:check

# Scan dependencies for known security vulnerabilities (govulncheck)
mise run vuln
```

---

## 11. Production Containerization & Deployment

Production containers use a minimal, multi-stage `scratch` image to minimize attack surface:

- Binaries are compiled with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags="-s -w"`.
- The runtime image contains no shell, package manager, or unnecessary utilities.
- The process runs as non-root user `nonroot` (UID: `10001`).

```bash
# Build production container image
mise run docker:build

# Run production container locally
docker run --rm -p 8081:8081 \
  -e APP_ENV=production \
  -e DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=require" \
  go-worker-template:latest
```

### Graceful Shutdown Lifecycle

Upon receiving `SIGINT` or `SIGTERM`:
1. The readiness probe (`/readyz`) flips to unhealthy (`503 Service Unavailable`).
2. The message listener halts accepting new incoming messages.
3. In-flight worker goroutines are allowed up to `SHUTDOWN_TIMEOUT` (default `30s`) to complete.
4. Database connection pool and message broker clients are closed cleanly before termination.
---

## 12. Adopting the Template for a New Service

To instantiate and rename this template for a new business domain:

```bash
mise run template:init github.com/your-org/your-worker
```

This task automatically:
1. Updates the module path in `go.mod`.
2. Replaces all module imports across `.go`, `.yaml`, `.toml`, and `Dockerfile` files.
3. Runs `mise run generate` to recreate all generated code.
4. Formats imports via `mise run format`.
5. Executes the test suite (`mise run test`) to verify synchronization.

---

## 13. Troubleshooting & FAQ

### 1. `bind: address already in use` (Port :8081 or :5432 is occupied)
- **Cause:** A stale worker process, Air instance, Delve session, or host database is holding the port.
- **Resolution:**
  ```bash
  # Identify and terminate process holding probe port 8081
  lsof -i :8081
  kill -9 <PID>

  # Check containers listening on port 5432
  docker ps --filter "publish=5432"
  ```

### 2. `failed to connect to PostgreSQL database`
- **Cause:** PostgreSQL container is not yet ready or credentials in `.env` do not match.
- **Resolution:**
  ```bash
  # Check container health status
  docker compose ps
  # Ensure container is started and healthy
  mise run docker:compose-up
  ```

### 3. `relation "tasks" does not exist`
- **Cause:** PostgreSQL container is active but database migrations have not been applied.
- **Resolution:**
  ```bash
  mise run migrate:up
  ```

### 4. Git Commit Blocked by Lefthook
- **Cause:** Unformatted Go files or linting violations in staged changes.
- **Resolution:**
  ```bash
  mise run format
  mise run lint:fix
  ```

---

## 14. Command Reference (Cheatsheet)

| Category | Command | Description |
| :--- | :--- | :--- |
| **Execution** | `mise run dev` | Starts dependencies, applies migrations, and launches worker with Air live-reload |
| | `go run ./cmd/worker` | Starts worker directly |
| **Database** | `mise run docker:compose-up` | Starts local PostgreSQL, RabbitMQ, and Redpanda containers |
| | `mise run docker:compose-down` | Stops and removes local containers and volumes |
| | `mise run migrate:up` | Applies pending schema migrations |
| | `mise run migrate:down` | Rolls back the latest schema migration |
| | `mise run migrate:status` | Displays migration status |
| | `mise run psql` | Connects directly to PostgreSQL CLI |
| **Code Generation** | `mise run generate` | Runs all code generators (SQLC, Mocks) |
| | `mise run generate:sqlc` | Generates type-safe SQL persistence code |
| | `mise run generate:mocks` | Generates interface mocks via Mockery |
| **Testing & Quality**| `mise run test` | Runs unit, client, and architecture tests with `-race` |
| | `mise run test:e2e` | Runs end-to-end Ginkgo test suite |
| | `mise run test:arch` | Verifies Hexagonal boundary rules |
| | `mise run test:coverage` | Computes test coverage percentage |
| | `mise run test:mutation` | Runs Gremlins domain mutation tests |
| | `mise run lint` | Runs full golangci-lint analysis |
| | `mise run lint:fix` | Automatically fixes linter findings |
| | `mise run format` | Formats Go files with `goimports` |
| | `mise run secret:check` | Scans repository for leaked secrets (Gitleaks) |
| | `mise run vuln` | Scans dependencies for known CVEs (govulncheck) |
| **Container** | `mise run docker:build` | Builds minimal scratch production container |
| **Documentation** | `mise run docs:serve` | Serves MkDocs documentation site locally |
| | `mise run docs:build` | Builds production MkDocs static site |
