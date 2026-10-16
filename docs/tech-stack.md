# Technology Stack & Toolchain Reference

This document outlines the technologies, libraries, and developer tools chosen for this service, detailing the architectural rationale, design trade-offs, and benefits of each component.

---

## 1. Architectural Summary

| Category | Technology | Version | Purpose / Role |
| :--- | :--- | :--- | :--- |
| **Language & Runtime** | [Go](https://go.dev/) | 1.27+ | Strongly-typed, garbage-collected systems language |
| **HTTP Router** | [Chi](https://github.com/go-chi/chi) | v5.3 | Lightweight, standard-library compliant HTTP routing |
| **API Contract** | [OpenAPI 3.0](https://spec.openapis.org/oas/v3.0.3) | 3.0.3 | Spec-first single source of truth for REST interfaces |
| **Code Generation** | [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) | v2 | Type-safe server handlers & schemas from OpenAPI |
| **RPC Framework** | [gRPC](https://grpc.io/) | v1.83 | High-performance, binary RPC protocol with HTTP/2 transport |
| **Protobuf Tooling** | [Buf CLI](https://buf.build/) | v1.50 | Hermetic Protobuf linter, breaking change detector & code generator |
| **gRPC CLI Client** | [grpcurl](https://github.com/fullstorydev/grpcurl) | v1.9 | Command-line tool for interacting with and probing gRPC servers |
| **Message Brokers** | [RabbitMQ](https://www.rabbitmq.com/) / [Redpanda](https://redpanda.com/) (Kafka) | Latest | Asynchronous message queues and event streaming |
| **Kafka Client** | [franz-go](https://github.com/twmb/franz-go) | v1.22 | Pure Go, high-performance Kafka & Redpanda client (zero CGo) |
| **Database Engine** | [PostgreSQL](https://www.postgresql.org/) | 16+ | Relational data persistence |
| **Database Driver** | [pgx](https://github.com/jackc/pgx) | v5.11 | High-performance, pure Go PostgreSQL driver & pool |
| **SQL Compiler** | [sqlc](https://sqlc.dev/) | v1.28 | Type-safe Go code generation directly from raw SQL |
| **Migrations** | [Goose](https://github.com/pressly/goose) | v3.24 | Versioned SQL schema migrations |
| **Identity / Keys** | [UUIDv7](https://github.com/google/uuid) | v1.6 | Time-ordered, B-Tree index-friendly primary keys |
| **Configuration** | [caarlos0/env](https://github.com/caarlos0/env) | v11.4 | Struct-tag declarative environment variable decoding |
| **Validation** | [validator](https://github.com/go-playground/validator) | v10.30 | Declarative struct-level validation rules |
| **Toolchain Manager**| [Mise](https://mise.jdx.dev/) | Latest | Hermetic runtime & CLI tool version management |
| **Hot Reloading** | [Air](https://github.com/air-verse/air) | v1.61 | Zero-restart rapid local iteration |
| **Debugger** | [Delve](https://github.com/go-delve/delve) | v1.24 | Production-grade Go runtime debugger |
| **Linter** | [golangci-lint](https://golangci-lint.run/) | v2 | Multi-linter static analysis aggregator |
| **Unit Testing** | [gotestsum](https://github.com/gotestyourself/gotestsum) / [testify](https://github.com/stretchr/testify) | Latest | Structured test execution & assertions with `-race` |
| **Mocking** | [Mockery](https://github.com/vektra/mockery) | v3 | Type-safe interface mock generator |
| **E2E Testing** | [Ginkgo](https://onsi.github.io/ginkgo/) & [Gomega](https://onsi.github.io/gomega/) | v2 | BDD expressive end-to-end black-box verification |
| **Secret Detection** | [Gitleaks](https://github.com/zricethezav/gitleaks) | v8 | Hardcoded credential and secret scanning |
| **Vulnerability** | [Govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) | Latest | Official Go vulnerability scanner |
| **Container Engine** | [Docker](https://www.docker.com/) / Buildx | Latest | Scratch-based, rootless, minimal attack surface image |
| **Image Security** | [Trivy](https://github.com/aquasecurity/trivy) | 0.60 | Rootless CVE scanning for container images |
| **Supply Chain** | [Cosign](https://github.com/sigstore/cosign) | Latest | Keyless container image signing via OIDC |

---

## 2. Technology Choices & Detailed Rationale

### 2.1. HTTP Layer: Chi (v5) vs. Gin / Fiber
- **Standard Library Compliance**: Chi router implements standard `http.Handler` and `http.HandlerFunc`. It introduces no custom context abstractions, allowing seamless interoperability with the entire Go standard library ecosystem.
- **Zero Reflection Overhead**: Chi performs pure radix-tree path matching with minimal memory allocation.
- **Spec-First Compatibility**: `oapi-codegen` natively produces Chi-compatible router adapters (`HandlerWithOptions`).
- **Why not Gin or Fiber?**: Gin relies on custom context wrappers (`*gin.Context`), making middleware reuse with standard libraries clunky. Fiber relies on `fasthttp`, which deviates from standard `net/http` patterns and HTTP/2 semantics.

### 2.2. API Contract: Spec-First (oapi-codegen) vs. Code-First (Swaggo)
- **Single Source of Truth**: The OpenAPI 3.0 specification (`api/openapi.yaml`) is the master contract agreed upon before code is written.
- **Strict Server Interfaces**: Handlers are forced to implement strongly-typed interfaces generated from the spec (`StrictServerInterface`). Handlers cannot return invalid status codes or unexpected payloads.
- **Why not Code-First (Swaggo comments)?**: Comment-based doc generation drifts easily from real handler logic, produces fragile annotations, and lacks strict request/response compile-time enforcement.

### 2.3. Persistence: sqlc + pgx/v5 vs. ORMs (GORM, Ent)
- **Raw SQL Control**: Developers write pure, idiomatic PostgreSQL queries in `database/queries/*.sql`. `sqlc` parses and compiles SQL into type-safe, idiomatic Go structs and methods.
- **Compile-Time Validation**: Query syntax, parameter types, and column references are verified at build time.
- **Zero Reflection & N+1 Prevention**: No runtime reflection, no magical lazy-loading surprises, and deterministic query performance.
- **Direct pgx/v5 Support**: sqlc directly targets `pgx/v5` connection pools (`pgxpool.Pool`), taking advantage of PostgreSQL binary protocol, native type support, and connection resilience.
- **Why not GORM?**: ORMs hide SQL complexity, introduce significant reflection overhead, make complex joins or database-specific optimizations awkward, and frequently cause N+1 query regressions.

### 2.4. Primary Keys: UUIDv7 vs. UUIDv4 vs. Auto-Increment (BIGINT)
- **Time-Ordered (K-Sortable)**: UUIDv7 embeds a millisecond-precision timestamp in the most significant bits.
- **B-Tree Index Friendly**: Unlike random UUIDv4 (which causes severe index fragmentation and random page splits on insertion), UUIDv7 inserts sequentially at the end of the B-tree leaf pages, yielding performance comparable to sequential integers.
- **Distributed & Unguessable**: Avoids sequential integer ID enumeration vulnerabilities (IDOR) while remaining globally unique without coordination across distributed instances.

### 2.5. Configuration: caarlos0/env/v11 vs. Viper
- **Simple, Fast, and Safe**: Reads configuration strictly from environment variables into a strongly typed Go struct using struct tags.
- **Fail-Fast Startup**: Validated on startup via `validator/v10`. If a port is out of range or a database URL is missing, the service aborts immediately with an actionable error.
- **Why not Viper?**: Viper has a massive dependency footprint, merges configuration across multiple conflicting sources (files, remote stores, flags, envs), and relies on untyped `Get(key)` string interfaces prone to runtime typos.

### 2.6. Local Toolchain: Mise vs. Makefile / asdf
- **Hermetic Tool Pinning**: Pins exact Go, linter, generator, and CLI versions in `mise.toml` and `go.mod` `tool` directives. Every developer and CI pipeline runs identical binaries.
- **Unified Task Orchestration**: Replaces complex Makefiles with declarative, self-documenting task definitions (`mise run dev`, `mise run test`, `mise run generate`).
- **Automatic Environment Injection**: Reads `.env` safely without external shell dependencies or wrappers.

### 2.7. Local Developer Experience: Air & Delve
- **Air Live-Reloading**: Automatically detects file changes, recompiles the binary, and restarts the server in milliseconds (`mise run dev`).
- **VS Code & Delve Integration**: Pre-configured launch profiles (`.vscode/launch.json`) support direct debugging, breakpoint stopping, and variable inspection for both the server and tests.

### 2.8. Security & Supply Chain: Scratch Containers, Trivy, and Cosign
- **Scratch Production Image**: Built from `scratch` containing only the compiled static Go binary, TLS certificates (`ca-certificates`), and non-root `/etc/passwd`. Contains zero shell, zero package manager, and zero operating system vulnerabilities.
- **Least Privilege User**: Runs as `USER 65532:65532` (`nonroot`), preventing root escalation attacks.
- **Rootless Trivy Scan**: Local container image tarball scanned for `CRITICAL` and `HIGH` CVEs before pushing.
- **Cosign Keyless Signing & SLSA**: Container images pushed to GHCR are signed using GitHub OIDC tokens, producing tamper-evident cryptographic provenance (SLSA v1) and SBOM attestations.
