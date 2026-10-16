# ---- Builder ----
FROM golang:1.27-alpine@sha256:738d1cf061836894ff6bb8c33881080ac66de8cf0586615012a0c8f592649cfa AS builder

WORKDIR /app

# Non-root user and group definitions (for scratch/unprivileged runtime)
RUN echo "appuser:x:10001:10001:appuser:/:/sbin/nologin" > /tmp/passwd && \
    echo "appgroup:x:10001:" > /tmp/group

# Download all dependencies for cache
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build application worker with -trimpath
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /bin/worker ./cmd/worker

# Build goose migration binary with -trimpath
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /bin/goose github.com/pressly/goose/v3/cmd/goose

# ---- Migration Runner Target (CI/CD & E2E) ----
FROM alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8 AS migration

RUN apk --no-cache add ca-certificates

COPY --from=builder /tmp/passwd /tmp/group /etc/
COPY --from=builder /bin/goose /bin/goose
COPY database/migrations /database/migrations

USER 10001:10001

ENTRYPOINT ["/bin/goose"]

# ---- Production Runtime (Default Stage: Scratch) ----
FROM scratch AS production

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /tmp/passwd /tmp/group /etc/
COPY --from=builder /bin/worker /worker
COPY --from=builder /bin/goose /bin/goose
COPY database/migrations /database/migrations
USER 10001:10001
EXPOSE 8081

ENTRYPOINT ["/worker"]
