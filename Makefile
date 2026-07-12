.PHONY: all build axiosd web clean dev test test-web vet start stop

SOCKET_DIR ?= /tmp/axios-mcp

all: build

# Build all Go binaries
build:
	go build -o bin/axiosd ./cmd/axiosd
	go build -o bin/axios-fs ./cmd/axios-fs
	go build -o bin/axios-system ./cmd/axios-system
	go build -o bin/axios-docker ./cmd/axios-docker

# Build individual binaries
axiosd:
	go build -o bin/axiosd ./cmd/axiosd

# Web UI
web:
	cd web && npm install && npm run build

# Development mode — MCP servers + axiosd + web UI
dev-mcp:
	@mkdir -p $(SOCKET_DIR)
	@echo "Starting MCP servers..."
	@go run ./cmd/axios-fs --socket $(SOCKET_DIR)/axios-fs.sock &
	@go run ./cmd/axios-system --socket $(SOCKET_DIR)/axios-system.sock &
	@go run ./cmd/axios-docker --socket $(SOCKET_DIR)/axios-docker.sock &
	@echo "MCP servers started. Sockets in $(SOCKET_DIR)/"

dev-axiosd:
	@mkdir -p $(SOCKET_DIR)
	go run ./cmd/axiosd --config configs/axiosd.yaml

dev-web:
	cd web && npm run dev

# Start everything with one command
start:
	@./scripts/axios start

stop:
	@./scripts/axios stop

# Run tests
test:
	go test ./... -race

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

vet:
	go vet ./...

lint:
	golangci-lint run

# Docker dev environment
dev-up:
	docker compose -f deploy/docker-compose.yml up --build

dev-down:
	docker compose -f deploy/docker-compose.yml down
