# Stage 1: Build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /build

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build both binaries.
COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION} -X github.com/fonzygrok/fonzygrok/internal/server.Version=${VERSION}" \
    -o /out/fonzygrok-server ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/fonzygrok-client ./cmd/client

# Stage 2: Runtime
FROM alpine:3.19

# Install ca-certificates and sqlite libs for CGO.
RUN apk add --no-cache ca-certificates

# Create non-root user.
RUN adduser -D -u 10001 fonzygrok

# Copy binaries.
COPY --from=builder /out/fonzygrok-server /usr/local/bin/fonzygrok-server
COPY --from=builder /out/fonzygrok-client /usr/local/bin/fonzygrok-client

# Copy entrypoint script (TLS toggle via TLS_ENABLED env var).
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh

# Create data and certs directories.
RUN mkdir -p /data /data/certs && chown -R fonzygrok:fonzygrok /data

# Expose ports:
#   2222  - SSH (client connections)
#   8080  - HTTP edge (tunnel proxy)
#   443   - HTTPS edge (tunnel proxy + dashboard, when TLS enabled)
#   9090  - Admin API (internal, bound to 0.0.0.0 in container)
#   40000-40100 - TCP tunnel port range (each TCP tunnel gets one port)
EXPOSE 2222 8080 443 9090 40000-40100

# Volumes for persistent data (database, host key, TLS certs).
VOLUME /data

# Run as non-root.
USER fonzygrok

ENTRYPOINT ["entrypoint.sh"]
CMD []
