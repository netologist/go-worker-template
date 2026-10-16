# ADR-0007: gRPC Protocol Support and Dual-Server Architecture

## Status
Accepted

## Date
2026-10-11

## Context
The service currently exposes its operations exclusively over HTTP/REST using an OpenAPI 3.0 specification-first design (ADR-0002) served via Chi. While this model provides broad accessibility, built-in Swagger UI documentation, and JSON interoperability, modern distributed microservice topologies require high-performance, low-latency, and type-safe inter-service remote procedure calls (RPC).

Integrating gRPC into an existing Go hexagonal service introduces several architectural challenges:

1. **Serving Model & Lifecycle Divergence**:
   Running gRPC must not undermine existing HTTP capabilities (such as `/healthz`, `/readyz`, OpenAPI docs at `/docs`, and existing REST consumers). The runtime must support running both protocols concurrently while preserving Kubernetes zero-downtime graceful shutdown coordination.
2. **Tooling & Hermetic Environment**:
   Standard gRPC compilation frequently relies on system-installed C++ `protoc` binaries, violating the hermetic, zero-global-dependency development environment established in ADR-0001.
3. **Public SDK & Package Encapsulation**:
   External clients require programmatic Go SDKs for both REST and gRPC. Go language rules forbid external packages from importing `internal/` paths. Generated Protobuf models and gRPC client stubs must be publicly consumable without forcing heavy gRPC transitive dependencies onto REST-only consumers.
4. **Error Handling & Layer Invariants**:
   In accordance with ADR-0005 (Layer Simplification and Zero-Ceremony Mapping), transport adapters must directly map shared domain invariants (`domain.ErrInvalidItem`, `domain.ErrItemAlreadyExists`) to protocol-specific status codes without redundant translation layers or duplicate schema validation frameworks.
5. **Architectural Isolation & Testability**:
   Transport adapters (`internal/adapter/httpapi` and `internal/adapter/grpc`) must remain strictly isolated from one another and independently testable via high-speed in-memory conduits (`bufconn`) and end-to-end suites.

---

## Decision

We adopt a **Dual-Protocol Serving Architecture** supporting concurrent HTTP/REST and gRPC servers backed by **Buf CLI** code generation, **`grpc.health.v1`** lifecycle synchronization, and decoupled client packages under `pkg/client/`.

### 1. Overall Dual-Protocol Architecture

The service coordinates both HTTP and gRPC transport layers within `cmd/server/main.go` using `golang.org/x/sync/errgroup`:

```mermaid
graph TD
    subgraph ClientLayer ["Consumers & Clients"]
        HTTPClient["HTTP/REST Consumers<br/>(Browsers, Ingress, curl)"]
        GRPCClient["gRPC Service Clients<br/>(Microservices, grpcurl)"]
        K8sProbe["Kubernetes Kubelet<br/>(Liveness & Readiness Probes)"]
    end

    subgraph Entrypoint ["cmd/server (Runtime Coordinator)"]
        CLI["CLI Flags & Env Config<br/>--server-mode=all|grpc|rest<br/>SERVER_MODE, HTTP_PORT, GRPC_PORT"]
        ErrGroup["errgroup.Group"]
        DrainCoord["Graceful Drain Coordinator<br/>(2-Step Shutdown: Unready -> Drain -> Stop)"]
    end

    subgraph Adapters ["Transport Adapters (internal/adapter)"]
        subgraph HTTPAdapter ["HTTP Adapter (:8080)"]
            Router["Chi Router & OpenAPI Mux"]
            HTTPHealth["HTTP Health Handler<br/>(/healthz, /readyz)"]
            SwaggerUI["Swagger UI & Docs<br/>(/docs, /openapi.json)"]
            HTTPItemHandler["Item HTTP Handler"]
        end

        subgraph GRPCAdapter ["gRPC Adapter (:50051)"]
            GRPCServer["grpc.Server"]
            GRPCHealth["grpc.health.v1<br/>HealthServer"]
            Reflection["gRPC Reflection Server"]
            GRPCItemHandler["Item gRPC Handler<br/>(ItemServiceServer)"]
        end
    end

    subgraph Core ["Application Core"]
        Usecase["internal/usecase<br/>ItemService"]
        Domain["internal/domain<br/>Item & Domain Errors"]
    end

    HTTPClient -->|HTTP/1.1 or HTTP/2| Router
    K8sProbe -->|GET /readyz| HTTPHealth
    K8sProbe -->|grpc_health_v1 Check| GRPCHealth
    GRPCClient -->|HTTP/2 gRPC| GRPCServer

    CLI --> ErrGroup
    ErrGroup --> Router
    ErrGroup --> GRPCServer
    ErrGroup --> DrainCoord

    DrainCoord -.->|SetReady false| HTTPHealth
    DrainCoord -.->|SetServingStatus NOT_SERVING| GRPCHealth

    Router --> HTTPItemHandler
    GRPCServer --> GRPCItemHandler
    HTTPItemHandler --> Usecase
    GRPCItemHandler --> Usecase
    Usecase --> Domain
```

- **Default Execution Mode**: `--server-mode=all` (both protocols run concurrently).
- **Alternative Modes**: `--server-mode=grpc` (gRPC only) or `--server-mode=rest` (HTTP/REST only).
- **Environment Variables**: Overridable via `SERVER_MODE`, `HTTP_PORT` (default `8080`), and `GRPC_PORT` (default `50051`).

---

### 2. Specification-First Protobuf Tooling Pipeline (Buf CLI)

Protobuf compilation is managed via **Buf CLI** pinned in `mise.toml` to prevent any requirement for system-installed `protoc` binaries:

```mermaid
graph LR
    subgraph Spec ["Contract Specification"]
        Proto["api/proto/item/v1/item.proto<br/>(syntax = 'proto3')"]
        BufYaml["buf.yaml<br/>(Lint & Breaking Rules)"]
        BufGen["buf.gen.yaml<br/>(Code Generation Rules)"]
    end

    subgraph Tooling ["Hermetic Toolchain"]
        MiseBuf["Buf CLI (mise.toml)<br/>mise run generate:proto"]
        GoPlugins["Go Tool Directives (go.mod)<br/>protoc-gen-go<br/>protoc-gen-go-grpc"]
    end

    subgraph Targets ["Generated Public Packages"]
        PbGo["pkg/proto/item/v1/item.pb.go<br/>(Protobuf Structs & Serialization)"]
        GrpcGo["pkg/proto/item/v1/item_grpc.pb.go<br/>(Client & Server Interfaces)"]
    end

    Proto --> MiseBuf
    BufYaml --> MiseBuf
    BufGen --> MiseBuf
    GoPlugins --> MiseBuf
    MiseBuf --> Targets
```

- **Specification Location**: `api/proto/item/v1/item.proto`.
- **Generated Target**: `pkg/proto/item/v1/`.
- **Generation Command**: `mise run generate:proto` (integrated into composite `mise run generate`).

---

### 3. Graceful Shutdown & Health Synchronization

To guarantee zero dropped requests during rolling Kubernetes upgrades, HTTP and gRPC readiness states are linked into a synchronized two-step drain cycle:

```mermaid
sequenceDiagram
    autonumber
    participant K8s as Kubernetes Kubelet
    participant Drain as Drain Coordinator (cmd/server)
    participant HTTPH as HTTP Health (/readyz)
    participant GRPCH as gRPC Health (grpc.health.v1)
    participant HTTPSrv as HTTP Server (:8080)
    participant GRPCSrv as gRPC Server (:50051)

    Note over K8s,GRPCSrv: Normal Operation (Serving Traffic)
    K8s->>HTTPH: GET /readyz (200 OK)
    K8s->>GRPCH: Check (SERVING)

    Signal->>Drain: SIGINT / SIGTERM Received
    Note over Drain: Step 1: Flip Readiness (Stop Ingress Routing)
    Drain->>HTTPH: SetReady(false) -> /readyz returns 503
    Drain->>GRPCH: SetServingStatus("", NOT_SERVING)

    Note over Drain: Step 2: Connection Drain Window (SERVER_DRAIN_TIMEOUT)
    Drain->>Drain: Sleep(ServerDrainTimeout) (Allows in-flight proxy updates)

    Note over Drain: Step 3: Server Graceful Shutdown
    par Graceful Stop
        Drain->>HTTPSrv: Shutdown(ctx with ServerShutdownTimeout)
        HTTPSrv-->>Drain: HTTP Connections Drained
    and
        Drain->>GRPCSrv: GracefulStop()
        GRPCSrv-->>Drain: gRPC RPCs Finished
    end
    Note over Drain: Process Exits Cleanly (Code 0)
```

---

### 4. Zero-Ceremony Domain Mapping & Status Codes

Following ADR-0005, the gRPC handler directly consumes `usecase.ItemService` and maps canonical domain errors into standard gRPC status codes (`google.golang.org/grpc/codes`):

```mermaid
graph TD
    Request["gRPC Request<br/>(CreateItemRequest)"] --> Handler["internal/adapter/grpc/handler<br/>ItemHandler.CreateItem"]
    Handler --> Service["internal/usecase<br/>ItemService.CreateItem(ctx, name, desc, price)"]
    Service --> DomainCheck{"Domain Invariants<br/>(domain.NewItem)"}

    DomainCheck -->|ErrInvalidItem| Invalid["codes.InvalidArgument<br/>(status.Errorf)"]
    DomainCheck -->|ErrItemAlreadyExists| Conflict["codes.AlreadyExists<br/>(status.Errorf)"]
    DomainCheck -->|Unexpected DB Error| Internal["codes.Internal<br/>('internal server error' + slog.Error)"]
    DomainCheck -->|Success (domain.Item)| Success["codes.OK<br/>(toProtoItem -> CreateItemResponse)"]

    Invalid --> Client["gRPC Caller"]
    Conflict --> Client
    Internal --> Client
    Success --> Client
```

| Domain Error / Condition | HTTP REST Status | gRPC Status Code | Rationale |
| :--- | :--- | :--- | :--- |
| `domain.ErrInvalidItem` | `400 Bad Request` | `codes.InvalidArgument` | Caller provided malformed payload or violated entity invariants. |
| `domain.ErrItemAlreadyExists` | `409 Conflict` | `codes.AlreadyExists` | Resource with identical natural key already exists. |
| `domain.ErrItemNotFound` | `404 Not Found` | `codes.NotFound` | Target entity does not exist in the persistence store. |
| Unhandled / Persistence Errors | `500 Internal Server Error` | `codes.Internal` | Unexpected server failure; sensitive trace logged to `slog`, masked to client. |

---

### 5. Decoupled Public Client SDK (`pkg/client`)

To prevent dependency bloat for external API consumers, the public SDK isolates HTTP and gRPC implementations into separate subpackages:

```mermaid
graph TD
    subgraph RepoRoot ["github.com/netologist/go-worker-template"]
        subgraph PublicSDK ["pkg/client/ (Public Go SDK)"]
            RESTClient["pkg/client/client.gen.go<br/>NewClientWithResponses(baseURL, opts...)<br/>Dependencies: kin-openapi, net/http"]
            GRPCSubpkg["pkg/client/grpc/client.go<br/>NewItemClient(target, opts...)<br/>NewItemClientFromConn(cc)<br/>Dependencies: google.golang.org/grpc"]
        end
        
        subgraph PublicProto ["pkg/proto/ (Public Protobuf DTOs)"]
            ItemProto["pkg/proto/item/v1<br/>ItemServiceClient, CreateItemRequest, Item"]
        end
    end

    ExternalHTTPApp["External Consumer A<br/>(REST only)"] -->|imports| RESTClient
    ExternalGRPCApp["External Consumer B<br/>(gRPC only)"] -->|imports| GRPCSubpkg
    ExternalGRPCApp -->|imports| ItemProto
    GRPCSubpkg -.->|uses| ItemProto
```

- **REST Clients**: Import `github.com/netologist/go-worker-template/pkg/client` (zero gRPC imports).
- **gRPC Clients**: Import `github.com/netologist/go-worker-template/pkg/client/grpc` and `pkg/proto/item/v1`.

---

### 6. Verification and Test Architecture

The gRPC implementation is verified across three testing tiers:

```mermaid
graph LR
    subgraph Tier1 ["Tier 1: Unit & SDK Tests"]
        BufConn["In-Memory Pipe<br/>(grpc/test/bufconn)"]
        SDKTest["pkg/client/grpc/client_test.go"]
        HandlerTest["internal/adapter/grpc/handler/item_test.go"]
        SDKTest --> BufConn --> HandlerTest
    end

    subgraph Tier2 ["Tier 2: Architecture Isolation"]
        ArchTest["tests/arch/architecture_test.go<br/>TestArchitecture_AdapterIsolation"]
        ArchCheck{"Enforce Rules:<br/>grpc adapter cannot import postgres<br/>grpc adapter cannot import httpapi"}
        ArchTest --> ArchCheck
    end

    subgraph Tier3 ["Tier 3: Live End-to-End Suite"]
        E2E["tests/e2e/item_test.go (Ginkgo)"]
        LiveServer["Running Server Binary<br/>--server-mode=all"]
        E2E -->|Port 8080| LiveServer
        E2E -->|Port 50051| LiveServer
    end
```

- **In-Memory Speed (`bufconn`)**: Unit and client SDK tests run in milliseconds without opening local OS network sockets.
- **CI Contract Drift (`validate-proto`)**: GitHub Actions runs `buf generate` and validates `git diff --exit-code -- pkg/proto/`.
- **E2E Ginkgo Suite**: Exercises both live ports concurrently with PostgreSQL persistence.

---

## Consequences

### Positive
- **High-Performance RPC**: Enables low-latency, binary Protobuf inter-service communication with streaming readiness.
- **Zero-Downtime Reliability**: Standard `grpc.health.v1` integrates seamlessly with Kubernetes gRPC health probes and the existing graceful shutdown drain coordinator.
- **Hermetic Toolchain**: Zero system-level dependencies; version-pinned via `mise.toml` (`buf`) and `go.mod` `tool` directives.
- **Isolated Public SDK**: Consumers importing REST do not pull in heavy gRPC dependencies, and vice versa.
- **Zero-Ceremony Code Reuse**: Core usecase and domain logic (`internal/usecase`, `internal/domain`) are 100% reused across HTTP and gRPC without duplicate validation or DTO ceremony.

### Negative
- **Expanded Surface Area**: Requires maintaining both `api/openapi.yaml` and `api/proto/item/v1/item.proto` as contracts.
- **Dual Runtime Management**: `cmd/server/main.go` coordinates two listening server loops and failure channels in parallel.

### Neutral
- Service engineers must run both `mise run generate:oapi` and `mise run generate:proto` (or composite `mise run generate`) when adding new domain operations.

---

## Alternatives Considered

1. **Single-Port Multiplexing (`cmux`)**:
   - *Rejected*: Running HTTP/1.1 and gRPC over the same port adds connection inspection latency, complicates TLS termination, and creates hard-to-debug HTTP/2 multiplexing edge cases.
2. **Mutually Exclusive Single Mode Only (`--grpc` XOR `--rest`)**:
   - *Rejected*: In production Kubernetes environments, disabling HTTP completely breaks existing HTTP ingress, Prometheus scrapers, and Swagger UI endpoints. Dual-serving is the cloud-native standard.
3. **Protobuf-Level Validation (`protovalidate`)**:
   - *Rejected*: Violates ADR-0005. Domain models already enforce business invariants; duplicating validation rules inside `.proto` files creates contract drift and unnecessary external library dependencies.
4. **Generating Protobuf Code into `internal/gen/grpc`**:
   - *Rejected*: Go's `internal` visibility prevents `pkg/client` and external consumers from importing generated Protobuf messages and client stubs.

---

## References
- ADR-0001: Standardized Development Environment and Tooling Pipeline
- ADR-0002: OpenAPI Specification-First Architecture with oapi-codegen and Chi
- ADR-0004: Configuration Management Architecture with caarlos0/env
- ADR-0005: Layer Simplification and Zero-Ceremony Mapping Architecture
- [Buf CLI Documentation](https://buf.build/docs)
- [gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md)
- [Kubernetes gRPC Liveness/Readiness Probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#define-a-grpc-liveness-probe)
