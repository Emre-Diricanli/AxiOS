.PHONY: all build claused web clean dev test start stop

SOCKET_DIR ?= /tmp/axios-mcp

all: build

# Build all Go binaries
build:
	go build -o bin/claused ./cmd/claused
	go build -o bin/axios-fs ./cmd/axios-fs
	go build -o bin/axios-system ./cmd/axios-system

# Build individual binaries
claused:
	go build -o bin/claused ./cmd/claused

# Web UI
web:
	cd web && npm install && npm run build

# Development mode — MCP servers + claused + web UI
dev-mcp:
	@mkdir -p $(SOCKET_DIR)
	@echo "Starting MCP servers..."
	@go run ./cmd/axios-fs --socket $(SOCKET_DIR)/axios-fs.sock &
	@go run ./cmd/axios-system --socket $(SOCKET_DIR)/axios-system.sock &
	@echo "MCP servers started. Sockets in $(SOCKET_DIR)/"

dev-claused:
	@mkdir -p $(SOCKET_DIR)
	go run ./cmd/claused --config configs/claused.yaml

dev-web:
	cd web && npm run dev

# Start everything with one command
start:
	@./scripts/axios start

stop:
	@./scripts/axios stop

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
	rm -f $(SOCKET_DIR)/*.sock

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
