# Project Folder Structure & Architecture Layout

This project implements **Hexagonal Architecture** (also known as **Ports and Adapters** / **Clean Architecture**) in Go. The architecture enforces strict layer separation, dependency inversion, and business logic isolation from delivery mechanisms and persistent storage engines.

---

## 1. Architectural Philosophy & Dependency Rule

The core architectural invariant is the **Dependency Inversion Principle**:
- **Domain** is at the center: it has **zero external dependencies** (no message brokers, no database drivers, no ORM annotations, no frameworks).
- **Use Cases** orchestrate domain operations and define application behavior.
- **Adapters** (Consumer listeners, Probe HTTP, PostgreSQL, DLQ, etc.) live at the outer rim and depend inward on ports (interfaces) declared by the domain or application layers.
- **cmd/worker** acts as the *Composition Root*: it instantiates concrete implementations, injects dependencies, and configures the runtime lifecycle.

```mermaid
graph TD
    subgraph Driving_Adapters [Driving Adapters (Primary Inbound)]
        Consumer[Message Listeners: Outbox / RabbitMQ / Kafka]
        Probe[Health & Probe HTTP Server :8081]
    end

    subgraph Core [Application Core]
        UC[ProcessTaskUseCase]
        DOM[Task Domain Models, Invariants & Ports]
    end

    subgraph Driven_Adapters [Driven Adapters (Secondary Outbound)]
        PG[(PostgreSQL / pgx & sqlc: Tasks & Idempotency)]
        DLQ[Dead Letter Queue Publisher]
    end

    Consumer -->|Delivers EventMessage| UC
    UC -->|Operates on| DOM
    PG -->|Implements Ports of| DOM
    DLQ -->|Implements Ports of| DOM
    UC -->|Invokes Outbound Ports| DOM
```

---

## 2. Directory Tree Overview

```text
.
├── .air.toml                      # Hot-reload configuration for local development
├── .dockerignore                  # Docker build context exclusions
├── .editorconfig                  # Uniform indentation and formatting across IDEs
├── .env.example                   # Local environment variable template
├── .gitattributes                 # Git line-ending and diff configurations
├── .gitignore                     # Git tracking exclusions
├── .golangci.yml                  # Static analysis and linter configuration (golangci-lint)
├── .mockery.yaml                  # Mockery configuration for interface mocking
├── Dockerfile                     # Multi-stage production container build (scratch-based)
├── Dockerfile.e2e                 # Dedicated container build for CI/local E2E suites
├── README.md                      # Project documentation and quickstart guide
├── docker-compose.yml             # Local dependencies (PostgreSQL 16, RabbitMQ, Redpanda)
├── docker-compose.e2e.yml         # Isolated test environment for Ginkgo E2E suites
├── go.mod                         # Go module definition and tool dependencies
├── go.sum                         # Checksums for Go module dependencies
├── lefthook.yml                   # Git pre-commit and pre-push hooks
├── mise.toml                      # Hermetic toolchain and task runner orchestration
├── sqlc.yml                       # SQL-to-Go code generator configuration
│
├── cmd/                           # Application entrypoints
│   └── worker/                    # Main background worker binary
│       ├── main.go                # Composition root, worker pool, graceful shutdown
│       └── main_test.go           # Integration smoke tests for bootstrap and flags
│
├── database/                      # Database schema and persistence definitions
│   ├── migrations/                # Goose migration files (Raw SQL DDL)
│   │   └── 20261006111444_create_tasks_and_idempotency.sql
│   └── queries/                   # Raw SQL queries compiled by sqlc
│       ├── idempotency.sql
│       └── tasks.sql
├── docs/                          # Architecture, decisions, and system documentation
│   ├── decisions/                 # Architecture Decision Records (ADRs)
│   │   ├── 0001-development-environment.md
│   │   ├── 0002-openapi-spec-first-architecture.md
│   │   ├── 0003-postgresql-persistence-layer.md
│   │   ├── 0004-configuration-management.md
│   │   ├── 0005-layer-simplification-and-mapping.md
│   │   └── 0006-local-development-air-delve.md
│   ├── techdebts/                 # Tracked technical debts and resolutions
│   │   └── 0001-ci-cache-poisoning.md
│   ├── folder-structure.md        # Detailed directory breakdown (this document)
│   ├── tech-stack.md              # Technical stack, libraries, and tool justification
│   ├── pipeline.md                # CI/CD and container security pipeline architecture
│   └── template-guide.md          # Guide for using this repository as a service template
│
├── internal/                      # Private application code (not importable by external repos)
│   ├── adapter/                   # Outer adapters (infrastructure, transport, persistence)
│   │   ├── consumer/              # Pluggable inbound message listener adapters
│   │   │   ├── listener.go        # Unified MessageListener interface
│   │   │   ├── recovery.go        # Panic recovery middleware for message processing
│   │   │   ├── recovery_test.go   # Panic recovery unit tests
│   │   │   ├── kafka/             # Redpanda / Apache Kafka consumer (franz-go)
│   │   │   ├── outbox/            # PostgreSQL transactional outbox poller (SKIP LOCKED)
│   │   │   └── rabbitmq/          # RabbitMQ AMQP 0-9-1 consumer
│   │   ├── dlq/                   # Dead letter queue publisher adapter
│   │   │   ├── publisher.go       # Logger/fallback DLQ implementation
│   │   │   └── publisher_test.go
│   │   ├── postgres/              # PostgreSQL persistence adapter
│   │   │   ├── postgres.go        # pgxpool connection lifecycle and health check
│   │   │   ├── postgres_test.go   # Connection error and ping tests
│   │   │   ├── idempotency_store.go # IdempotencyStore implementation using pgxpool & sqlc
│   │   │   ├── task_repository.go # TaskRepository implementation using pgxpool & sqlc
│   │   │   └── task_converter.go  # Database model to domain entity mapping
│   │   └── probe/                 # Lightweight HTTP health & probe server (:8081)
│   │       ├── server.go          # /livez and /readyz HTTP endpoints
│   │       └── server_test.go     # Probe endpoint unit tests
│   ├── config/                    # Configuration management and validation
│   │   ├── config.go              # Env struct tags mapped with caarlos0/env/v11
│   │   ├── config_test.go         # Default value and validation failure tests
│   │   └── validate.go            # Custom struct validation using go-playground/validator
│   │
│   ├── domain/                    # Pure domain models and ports (interfaces)
│   │   ├── errors.go              # Sentinel domain errors (ErrTransient, ErrFatal, etc.)
│   │   ├── message.go             # Inbound EventMessage value object
│   │   ├── ports.go               # Inbound and outbound port interfaces
│   │   ├── task.go                # Task domain entity and business invariants
│   │   └── task_test.go           # Domain constructor and invariant unit tests
│   │
│   ├── gen/                       # Generated code (NEVER edit manually)
│   │   ├── mocks/domain/          # Auto-generated unit test mocks (Mockery)
│   │   └── sqlc/                  # Auto-generated type-safe PostgreSQL queries
│   │
│   └── usecase/                   # Application business use cases
│       ├── process_task.go        # ProcessTaskUseCase with idempotency & error classification
│       └── process_task_test.go   # Use case unit tests using domain mocks
├── tests/                         # Integration, Architecture, and End-to-End suites
│   ├── arch/                      # Hexagonal Architecture layer boundary enforcement tests
│   │   └── architecture_test.go   # Pure Go parser tests verifying layer isolation
│   └── e2e/                       # Ginkgo v2 & Gomega BDD end-to-end tests
│       ├── e2e_suite_test.go      # Ginkgo suite bootstrap and lifecycle hooks
│       └── probe_test.go          # Health probe & task processing E2E scenarios
│
└── .vscode/                       # VS Code workspace settings and debugging launches
    ├── launch.json                # Delve debug targets (server, unit tests, E2E)
    ├── settings.json              # Go extension and linter configurations
    └── extensions.json            # Recommended developer extensions
```

---

## 3. Layer Breakdown & Responsibilities

### 3.1. `cmd/worker/` (Composition Root)
- The single entry point for the worker process.
- **Responsibilities**:
  1. Loads and validates environment variables via `internal/config`.
  2. Initializes structured logging (`slog`) and observability.
  3. Initializes the database connection pool (`internal/adapter/postgres`).
  4. Wires concrete adapters to use case implementations and starts inbound `MessageListener`.
  5. Runs worker pool concurrency managed via `golang.org/x/sync/errgroup`.
  6. Launches isolated probe server on port `8081` for Kubernetes `/livez` and `/readyz`.
  7. Listens for OS termination signals (`SIGINT`, `SIGTERM`) and coordinates graceful drain: stops consumer ingestion, drains active worker goroutines, finishes message acks, and safely closes database pools.

### 3.2. `internal/domain/` (The Core)
- Holds pure domain entities, value objects, and business rules.
- **Strict Invariants**:
  - Contains **zero third-party imports** (standard library only, except UUID if used as an identity primitive).
  - Domain constructors (`NewTask(...)`) validate business invariants (e.g., non-empty payload, valid task types, state transitions).
  - Declares **Inbound Ports**:
    - `TaskProcessor`: Contract for processing event messages.
  - Declares **Outbound Ports**:
    - `TaskRepository`: Contract for task state persistence.
    - `IdempotencyStore`: Contract for atomic message deduplication (`TryAcquire`, `MarkCompleted`, `MarkFailed`).
    - `DeadLetterPublisher`: Contract for routing poison pills to dead-letter storage.
  - Declares **Sentinel Domain Errors**: `ErrTransient`, `ErrFatal`, `ErrTaskNotFound`, `ErrMessageAlreadyProcessed`.

### 3.3. `internal/usecase/` (Application Layer)
- Implements application business logic and orchestrates domain operations.
- Interacts strictly with domain ports (`TaskRepository`, `IdempotencyStore`, `DeadLetterPublisher`).
- Implements deduplication, transient error classification, and dead-letter routing.
- Contains no transport or SQL details; unit-tested thoroughly using generated mocks from `internal/gen/mocks`.

### 3.4. `internal/adapter/consumer/` (Inbound Adapters)
- Consumes event messages from messaging brokers or polling queues:
  - `outbox/`: PostgreSQL transactional outbox polling using `FOR UPDATE SKIP LOCKED`.
  - `rabbitmq/`: RabbitMQ consumer using AMQP 0-9-1 channels and ACK/NACK semantics.
  - `kafka/`: Redpanda / Apache Kafka consumer using `franz-go` consumer groups.
  - `recovery.go`: Recovers from goroutine panics in message handlers to ensure worker process stability.

### 3.5. `internal/adapter/probe/` (Health Probes)
- Standalone HTTP server on port `8081` serving `/livez` and `/readyz` for Kubernetes container orchestration.

### 3.6. `internal/adapter/postgres/` (Driven Persistence Adapter)
- Implements `TaskRepository` and `IdempotencyStore` using `pgxpool.Pool` and `sqlc`-generated queries.
- Manages PostgreSQL error mapping and transactional integrity.

### 3.7. `internal/config/` (Configuration)
- Manages application configuration loaded strictly from environment variables using `caarlos0/env/v11`.
- Enforces runtime assertions on startup using `go-playground/validator/v10` (e.g., worker concurrency ranges, valid broker addresses, positive pool sizes).

### 3.8. `internal/gen/` (Generated Artifacts)
- Houses automatically generated code maintained under strict drift detection in CI:
  - `gen/sqlc/`: Type-safe Go query methods generated from `database/queries/*.sql`.
  - `gen/mocks/`: Interface mocks generated by `mockery` for unit testing.

### 3.9. `database/` (Data Schemas)
- `migrations/`: Versioned SQL migration files managed by Goose (`create_tasks_and_idempotency.sql`).
- `queries/`: Clean, hand-crafted SQL queries annotated with sqlc tags (`tasks.sql`, `idempotency.sql`).

### 3.10. `tests/e2e/` (Black-Box Verification)
- Ginkgo v2 and Gomega BDD tests that treat the running worker as a black box.
- Verifies probe status, queue ingestion, task execution, and idempotency guarantees against real PostgreSQL and broker containers.
