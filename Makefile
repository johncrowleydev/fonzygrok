.PHONY: build build-server build-client test test-e2e lint vet clean docker-build docker-up docker-down docker-logs

VERSION ?= dev
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"

## Build both binaries
build: build-server build-client

build-server:
	mkdir -p dist
	CGO_ENABLED=1 go build $(LDFLAGS) -o dist/fonzygrok-server ./cmd/server/

build-client:
	mkdir -p dist
	CGO_ENABLED=0 go build $(LDFLAGS) -o dist/fonzygrok ./cmd/client/

## Run all unit tests with race detection
test:
	CGO_ENABLED=1 go test -race ./...

## Run end-to-end integration tests
test-e2e:
	CGO_ENABLED=1 go test -v -race -tags=e2e -timeout 60s ./tests/

## Linting
lint: vet

vet:
	go vet ./...

## Clean build artifacts
clean:
	rm -f fonzygrok-server fonzygrok
	rm -rf bin/ dist/

## Build Docker image
docker-build:
	docker build -t fonzygrok-server:$(VERSION) -f docker/Dockerfile --build-arg VERSION=$(VERSION) .

## Start server in Docker
docker-up:
	docker compose -f docker/docker-compose.yml up -d

## Stop Docker containers
docker-down:
	docker compose -f docker/docker-compose.yml down

## View Docker logs
docker-logs:
	docker compose -f docker/docker-compose.yml logs -f
