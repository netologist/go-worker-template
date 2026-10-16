# Developer User Guide

This guide provides end-to-end instructions for developing, operating, testing, debugging, and deploying the **Go Service Template** microservice architecture (Hexagonal Architecture, OpenAPI spec-first code generation, PostgreSQL persistence with UUIDv7, hermetic `mise` toolchains, and zero-trust CI/CD standards).

---

## 1. Architecture & Core Capabilities

The service follows strict architectural boundaries designed for high maintainability, testability, and operational resilience:

- **Hexagonal Architecture (Ports & Adapters):** Core business logic (`internal/domain`) and application use cases (`internal/usecase`) are completely decoupled from external frameworks and database drivers. The HTTP API (`internal/adapter/httpapi`) and PostgreSQL storage (`internal/adapter/postgres`) operate as pluggable adapters.
- **Spec-First API Contract:** The single source of truth for the HTTP API is `api/openapi.yaml`. Strict typed router interfaces, models, request validators, and client SDKs are generated automatically via `oapi-codegen`.
- **Zero-Ceremony Model Mapping (ADR-0005):** Redundant intermediate use case DTOs are eliminated. Use cases accept domain primitives and return `domain.Item` values directly. Handlers map directly between OpenAPI contracts and domain entities.
- **Type-Safe Persistence (ADR-0003):** Compile-time SQL query validation with `sqlc`, version-controlled migrations with `goose`, high-performance connection pooling with `pgx/v5`, and native B-Tree-friendly `UUIDv7` primary keys.
- **Hermetic Toolchain (ADR-0001):** All development CLI tools (`air`, `goose`, `sqlc`, `oapi-codegen`, `mockery`, `golangci-lint`, `gotestsum`, `gitleaks`, `govulncheck`, `lefthook`) are pinned directly inside `go.mod` via Go's native `tool` directive and executed hermetically through `mise`.
- **RFC 9457 Problem Details:** All API errors return standard `application/problem+json` payloads containing unique, traceable `request_id` values.
- **Dual-Loop Local Development (ADR-0006):** Decoupled workflows provide fast live-reloading via `Air` (`mise run dev`) and interactive step debugging via native `Delve` inside VS Code (`F5`).

---

## 2. Prerequisites & Environment Setup

To run and develop this project, you only need two tools installed on your host machine:

| Tool | Minimum Version | Responsibility |
| :--- | :--- | :--- |
| **[Mise](https://mise.jdx.dev/)** | Latest | Manages Go runtime (`1.27.2`), Protobuf tooling (`buf`), gRPC CLI (`grpcurl`), and task orchestration. |
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

# Step 2: Start PostgreSQL container (waits until healthy)
mise run docker:compose-up

# Step 3: Apply database schema migrations
mise run migrate:up

# Step 4: Run HTTP server
go run ./cmd/server
```

The HTTP server starts listening on `http://localhost:8080`.

### Smoke Verification

In a new terminal window, verify the service endpoints:

```bash
# 1. Health and readiness check
curl -i http://localhost:8080/healthz

# Expected Response: HTTP/1.1 200 OK
# {"status":"ok","timestamp":"2026-10-10T..."}

# 2. Register a new item
curl -i -X POST http://localhost:8080/api/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name": "Mechanical Keyboard", "description": "Hot-swappable custom keyboard", "price": 14900}'

# Expected Response: HTTP/1.1 201 Created

# 3. List all registered items
curl -i http://localhost:8080/api/v1/items
```

---

## 4. Configuration & Environment Variables

Configuration is declared in `internal/config/config.go` and parsed from environment variables using `caarlos0/env/v11`. All settings are strictly validated on application startup via `go-playground/validator` (fail-fast principle).

Below is the complete reference of environment variables defined in `.env.example`:

| Environment Variable | Type / Format | Default | Description & Constraints |
| :--- | :--- | :--- | :--- |
| `APP_ENV` | `string` | `development` | Target environment (`development`, `staging`, `production`, `test`). In `production`, Swagger UI (`/docs/`) and `/openapi.json` are disabled. |
| `HTTP_PORT` | `int` | `8080` | Port for the HTTP server (`1`–`65535`). |
| `LOG_LEVEL` | `string` | `info` | Structured logging level (`debug`, `info`, `warn`, `error`). |
| `DATABASE_URL` | `string` | *(Required)* | PostgreSQL connection URI. Example: `postgres://postgres:S3cr3tP455w0rD@localhost:5432/service_db?sslmode=disable` |
| `CORS_ALLOWED_ORIGINS` | `csv` | Empty | Comma-separated list of allowed origins (e.g., `https://app.example.com,http://localhost:3000`). Wildcards (`*`) are rejected. If empty, CORS is disabled. |
| `SERVER_SHUTDOWN_TIMEOUT` | `duration` | `10s` | Maximum time allowed for in-flight requests to complete during shutdown. |
| `SERVER_DRAIN_TIMEOUT` | `duration` | `5s` | Drain wait duration after flipping readiness probe to allow ingress/K8s endpoints to update. |
| `SERVER_READ_HEADER_TIMEOUT`| `duration` | `30s` | Timeout for reading HTTP request headers (mitigates Slowloris attacks). |
| `SERVER_READ_TIMEOUT` | `duration` | `30s` | Timeout for reading the entire request body. |
| `SERVER_WRITE_TIMEOUT` | `duration` | `30s` | Timeout for writing the HTTP response. |
| `SERVER_IDLE_TIMEOUT` | `duration` | `2m` | Keep-alive idle connection timeout. |
| `DB_MAX_CONNS` | `int32` | `25` | Maximum number of active connections in the `pgxpool`. |
| `DB_MIN_CONNS` | `int32` | `2` | Minimum number of idle connections maintained in the pool. |
| `DB_MAX_CONN_LIFETIME` | `duration` | `30m` | Maximum lifetime of a database connection before retirement. |
| `DB_MAX_CONN_IDLE_TIME` | `duration` | `5m` | Maximum idle time before closing an unused database connection. |
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

1. If `mise run dev` is running, **stop it** to avoid port `:8080` collisions.
2. Ensure PostgreSQL is running: `mise run docker:compose-up`.
3. Open VS Code and switch to the **Run and Debug** view (`Ctrl+Shift+D` / `Cmd+Shift+D`).
4. Select **Debug Server (cmd/server)** and press **F5**.
5. Set breakpoints in handlers, use cases, or domain files. Send an HTTP request to trigger the debugger.

To debug a single test package, place your cursor in the target test file, select **Debug Current Test** in VS Code, and press **F5**.

---

## 6. Code Generation & Spec-First Workflow

All HTTP routing interfaces, models, client SDKs, SQL queries, and test mocks are generated from authoritative source files:

```mermaid
graph LR
    API["api/openapi.yaml"] -->|mise run generate:oapi| OpenAPIGen["internal/gen/openapi/api.gen.go"]
    API -->|mise run generate:client| ClientGen["pkg/client/client.gen.go"]
    SQL["database/queries/"] -->|mise run generate:sqlc| SQLCGen["internal/gen/sqlc/"]
    Interfaces["internal/.../*.go"] -->|mise run generate:mocks| MocksGen["internal/gen/mocks/"]
```

### Generator Commands

| Command | Description |
| :--- | :--- |
| `mise run generate` | Executes all generators in dependency order (OpenAPI Server, Client SDK, SQLC, Mockery). |
| `mise run generate:proto` | Compiles `api/proto/item/v1/item.proto` into `pkg/proto/item/v1/` using Buf CLI. |
| `mise run generate:oapi` | Compiles `api/openapi.yaml` into strict Chi server interfaces and request/response models. |
| `mise run generate:client` | Compiles `api/openapi.yaml` into a production-ready Go HTTP Client SDK under `pkg/client`. |
| `mise run generate:sqlc` | Compiles SQL queries in `database/queries/*.sql` into type-safe Go persistence methods. |
| `mise run generate:mocks` | Generates interface test mocks into `internal/gen/mocks/` via Mockery. |
| `mise run mocks:check` | Verifies in CI that generated mocks are up to date without unstaged drift. |

### Adding a New API Endpoint

1. Edit `api/openapi.yaml` to define the new path, parameters, request bodies, and responses (including RFC 9457 errors).
2. Run `mise run generate:oapi` and `mise run generate:client`.
3. Implement the newly generated `StrictServerInterface` method in `internal/adapter/httpapi/handler/`.
4. Implement corresponding business logic in `internal/usecase` and `internal/domain`.
5. Run `mise run generate:mocks && mise run lint && mise run test` to verify.

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

## 8. API Reference & RFC 9457 Problem Details

### 1. System & Diagnostic Endpoints

- **Liveness & Readiness Probe:**
  - `GET /healthz` $\to$ Returns `200 OK` when the service is healthy and the database is reachable. Returns `503 Service Unavailable` if database connectivity is lost or the service is shutting down.
- **Interactive Swagger UI:**
  - `GET http://localhost:8080/docs/`
  - Self-contained Swagger UI embedded directly inside the binary (no external CDN dependencies). Only accessible in `development` and `staging` environments.
- **Raw OpenAPI Specification:**
  - `GET http://localhost:8080/openapi.json`

### 2. Items API Endpoints

#### Create Item (`POST /api/v1/items`)

```bash
curl -X POST http://localhost:8080/api/v1/items \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Noise Cancelling Headphones",
    "description": "Wireless over-ear headphones",
    "price": 29900
  }'
```

**Success Response (HTTP 201 Created):**
```json
{
  "id": "019464e8-8f5b-7b0a-9d2a-4a6c8e312f10",
  "name": "Noise Cancelling Headphones",
  "description": "Wireless over-ear headphones",
  "price": 29900,
  "created_at": "2026-10-10T21:17:28.765Z",
  "updated_at": "2026-10-10T21:17:28.765Z"
}
```

#### List & Filter Items (`GET /api/v1/items`)

```bash
# List all items
curl http://localhost:8080/api/v1/items

# Filter items by name
curl "http://localhost:8080/api/v1/items?name=Headphones"
```

### 3. RFC 9457 Problem Details

All error responses return the standard `application/problem+json` media type:

#### Validation Error (HTTP 400 Bad Request)
```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "request body validation failed",
  "instance": "/api/v1/items",
  "code": "VALIDATION_ERROR",
  "request_id": "c1f7b889-1234-4567-89ab-cdef01234567",
  "errors": [
    {
      "name": "name",
      "reason": "must be at least 2 characters long"
    }
  ]
}
```

#### Conflict Error (HTTP 409 Conflict)
Attempting to create an item with an existing unique name:
```json
{
  "type": "about:blank",
  "title": "Conflict",
  "status": 409,
  "detail": "item with the specified name already exists",
  "instance": "/api/v1/items",
  "code": "ITEM_ALREADY_EXISTS",
  "request_id": "d2e8c990-2345-6789-01bc-def012345678"
}
```


### 4. gRPC API & grpcurl Reference

The gRPC server runs on port `:50051` with Server Reflection and `grpc.health.v1` enabled. You can interact with it using `grpcurl` (installed via Mise) or via Go tool directives (`go tool grpcurl`):

```bash
# 1. List all available services via reflection
grpcurl -plaintext localhost:50051 list
# Output:
# grpc.health.v1.Health
# grpc.reflection.v1.ServerReflection
# grpc.reflection.v1alpha.ServerReflection
# item.v1.ItemService

# 2. Inspect ItemService schema and RPC definitions
grpcurl -plaintext localhost:50051 describe item.v1.ItemService

# 3. Check server health status
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

# 4. Register a new item via gRPC
grpcurl -plaintext -d '{
  "name": "Mechanical Keyboard",
  "description": "Ergonomic hot-swappable keyboard",
  "price": 9900
}' localhost:50051 item.v1.ItemService/CreateItem

# 5. Query items via gRPC
grpcurl -plaintext -d '{"name": "Keyboard"}' localhost:50051 item.v1.ItemService/FindItems
```

---

## 9. Go Client SDK Usage (`pkg/client`)

External Go services can consume this API using the generated typed SDK:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/netologist/go-worker-template/pkg/client"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Initialize client
	c, err := client.NewClientWithResponses("http://localhost:8080")
	if err != nil {
		log.Fatalf("failed to initialize client: %v", err)
	}

	// 2. Health check
	healthResp, err := c.GetHealthWithResponse(ctx)
	if err != nil || healthResp.StatusCode() != 200 {
		log.Fatalf("service is unhealthy: %v", err)
	}

	// 3. Create a new item
	desc := "Ergonomic mechanical keyboard"
	createResp, err := c.CreateItemWithResponse(ctx, client.CreateItemJSONRequestBody{
		Name:        "Ergonomic Keyboard",
		Description: &desc,
		Price:       19900,
	})
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}

	if createResp.JSON201 != nil {
		fmt.Printf("Created item ID: %s, Name: %s\n", createResp.JSON201.Id, createResp.JSON201.Name)
	} else if createResp.JSON409 != nil {
		fmt.Printf("Item already exists: %s\n", *createResp.JSON409.Detail)
	}
}
```

---

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
docker run --rm -p 8080:8080 \
  -e APP_ENV=production \
  -e DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=require" \
  go-worker-template:latest
```

### Graceful Shutdown Lifecycle

Upon receiving `SIGINT` or `SIGTERM`:
1. The readiness probe (`/healthz`) flips to unhealthy (`503 Service Unavailable`).
2. The server waits `SERVER_DRAIN_TIMEOUT` (default `5s`) for ingress controllers to remove the instance from routing pools.
3. In-flight requests are allowed up to `SERVER_SHUTDOWN_TIMEOUT` (default `10s`) to complete.
4. The database connection pool is closed cleanly before process termination.

---

## 12. Adopting the Template for a New Service

To instantiate and rename this template for a new business domain:

```bash
mise run template:init github.com/your-org/your-service
```

This task automatically:
1. Updates the module path in `go.mod`.
2. Replaces all module imports across `.go`, `.yaml`, `.toml`, and `Dockerfile` files.
3. Runs `mise run generate` to recreate all generated code.
4. Formats imports via `mise run format`.
5. Executes the test suite (`mise run test`) to verify synchronization.

---

## 13. Troubleshooting & FAQ

### 1. `bind: address already in use` (Port :8080 or :5432 is occupied)
- **Cause:** A stale Air process, Delve debug session, or host PostgreSQL instance is holding the port.
- **Resolution:**
  ```bash
  # Identify and terminate process holding port 8080
  lsof -i :8080
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

### 3. `relation "items" does not exist`
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
| **Execution** | `mise run dev` | Starts PostgreSQL, applies migrations, and launches server with Air live-reload |
| | `go run ./cmd/server` | Starts HTTP server directly |
| **Database** | `mise run docker:compose-up` | Starts local PostgreSQL container |
| | `mise run docker:compose-down` | Stops and removes local PostgreSQL container and volumes |
| | `mise run migrate:up` | Applies pending schema migrations |
| | `mise run migrate:down` | Rolls back the latest schema migration |
| | `mise run migrate:status` | Displays migration status |
| | `mise run psql` | Connects directly to PostgreSQL CLI |
| **Code Generation** | `mise run generate` | Runs all code generators (OpenAPI server, Client SDK, SQLC, Mocks) |
| | `mise run generate:oapi` | Generates HTTP router interfaces and models |
| | `mise run generate:client` | Generates Go client SDK |
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
