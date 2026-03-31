# Stage 1: Build
FROM golang:1.22-alpine AS builder

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

# Create data directory.
RUN mkdir -p /data && chown fonzygrok:fonzygrok /data

# Expose ports: SSH, HTTP edge, Admin API.
EXPOSE 2222 8080 9090

# Volume for persistent data (database, host key).
VOLUME /data

# Run as non-root.
USER fonzygrok

ENTRYPOINT ["fonzygrok-server"]
CMD ["serve", "--data-dir", "/data", "--ssh-addr", ":2222", "--http-addr", ":8080", "--admin-addr", "0.0.0.0:9090"]
