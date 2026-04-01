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

# Create data and certs directories.
RUN mkdir -p /data /data/certs && chown -R fonzygrok:fonzygrok /data

# Expose ports: SSH, HTTP edge, HTTPS edge, Admin API.
EXPOSE 2222 8080 443 9090

# Volumes for persistent data (database, host key, TLS certs).
VOLUME /data

# Run as non-root.
USER fonzygrok

ENTRYPOINT ["fonzygrok-server"]
CMD ["serve", "--data-dir", "/data", "--ssh-addr", ":2222", "--http-addr", ":8080", "--admin-addr", "0.0.0.0:9090"]
