.PHONY: all build claused web clean dev test

all: build

# Build all Go binaries
build:
	go build -o bin/claused ./cmd/claused

# Build individual binaries
claused:
	go build -o bin/claused ./cmd/claused

# Web UI
web:
	cd web && npm install && npm run build

# Development mode
dev-claused:
	go run ./cmd/claused --config configs/claused.yaml

dev-web:
	cd web && npm run dev

# Run tests
test:
	go test ./...

test-web:
	cd web && npm test

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf web/dist/
	rm -rf web/node_modules/

# Format and lint
fmt:
	go fmt ./...

lint:
	golangci-lint run

# Docker dev environment
dev-up:
	docker compose -f deploy/docker-compose.yml up --build

dev-down:
	docker compose -f deploy/docker-compose.yml down
