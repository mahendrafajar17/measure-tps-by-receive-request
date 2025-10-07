# Makefile for TPS Calculator Webhook Server

BINARY_NAME = webhook-server
BIN_DIR = ./bin
SOURCE_DIR = ./
CONFIG_FILE = config.yaml

# Default Go build flags
GO_BUILD_FLAGS = -ldflags="-s -w"

# Build the binary for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@rm -f $(BINARY_NAME)
	@go build $(GO_BUILD_FLAGS) -o $(BINARY_NAME) $(SOURCE_DIR)main.go
	@echo "Binary created: $(BINARY_NAME)"

# Build for Linux (production deployment)  
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	@rm -f $(BINARY_NAME)
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) -o $(BINARY_NAME) $(SOURCE_DIR)main.go
	@echo "Linux binary created: $(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -f $(BINARY_NAME).log
	@rm -f $(BINARY_NAME).pid
	@rm -f webhook.log
	@echo "Clean completed"

# Run the application directly
run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BINARY_NAME)

# Start the application using launcher (production mode)
start:
	@echo "Starting $(BINARY_NAME) with launcher..."
	@./launch.sh start

# Start with existing binary (for production servers without Go)
start-prod:
	@echo "Starting $(BINARY_NAME) with existing binary..."
	@./launch.sh start

# Stop the application
stop:
	@./launch.sh stop

# Restart the application  
restart:
	@./launch.sh restart

# Check application status
status:
	@./launch.sh status

# Follow application logs
logs:
	@./launch.sh logs

# Monitor webhook metrics (real-time dashboard)
monitor:
	@echo "Starting webhook TPS monitor..."
	@./monitor.sh

# Reset all webhook metrics
reset-all:
	@echo "Resetting all webhook metrics..."
	@curl -s -X POST http://localhost:8080/api/webhooks/default/reset > /dev/null || echo "Failed to reset default"
	@curl -s -X POST http://localhost:8080/api/webhooks/fast/reset > /dev/null || echo "Failed to reset fast"
	@curl -s -X POST http://localhost:8080/api/webhooks/slow/reset > /dev/null || echo "Failed to reset slow"
	@curl -s -X POST http://localhost:8080/api/webhooks/medium/reset > /dev/null || echo "Failed to reset medium"
	@curl -s -X POST http://localhost:8080/api/webhooks/custom/reset > /dev/null || echo "Failed to reset custom"
	@echo "✅ All metrics reset completed"

# Show current metrics summary
metrics:
	@echo "📊 Current Webhook Metrics Summary:"
	@curl -s http://localhost:8080/api/summary | jq -r '.summary | to_entries[] | "  \(.value.name): \(.value.total_requests) requests, \(.value.tps | floor) TPS, \(.value.delay_ms)ms delay"' || echo "Server not running or jq not available"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	@golangci-lint run || echo "golangci-lint not installed, skipping..."

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Create example config if it doesn't exist
config:
	@if [ ! -f $(CONFIG_FILE) ]; then \
		echo "Creating example config file..."; \
		cp $(CONFIG_FILE) config.yaml.example 2>/dev/null || echo "Config file already exists"; \
	fi

# Development mode - run with auto-restart on file changes (requires air)
dev:
	@echo "Starting development mode..."
	@air -c .air.toml || echo "air not installed, use 'go install github.com/cosmtrek/air@latest'"

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@go install github.com/cosmtrek/air@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build binary for current platform"
	@echo "  build-linux  - Build binary for Linux (production)"
	@echo "  clean        - Clean build artifacts"
	@echo "  run          - Build and run application directly"
	@echo "  start        - Build and start application with launcher"
	@echo "  stop         - Stop application"
	@echo "  restart      - Restart application"
	@echo "  status       - Check application status"
	@echo "  logs         - Follow application logs"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  deps         - Install dependencies"
	@echo "  config       - Create example config"
	@echo "  dev          - Start development mode with auto-restart"
	@echo "  install-tools- Install development tools"
	@echo "  help         - Show this help message"

.PHONY: build build-linux clean run start stop restart status logs test test-coverage fmt lint deps config dev install-tools help