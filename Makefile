.PHONY: build build-server build-client test lint clean

VERSION ?= dev
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

build: build-server build-client

build-server:
	go build $(LDFLAGS) -o fonzygrok-server ./cmd/server/

build-client:
	go build $(LDFLAGS) -o fonzygrok ./cmd/client/

test:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -f fonzygrok-server fonzygrok
	rm -rf dist/
