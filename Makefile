.PHONY: run build test test-cover docker-up docker-down test-ollama

# Development run: start the server directly
run:
	go run ./cmd/server

# Build the binary
build:
	go build -o bin/modelgate ./cmd/server

# Run unit tests
test:
	go test ./... -v

# Run tests with coverage
test-cover:
	go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out

# Start Redis container
docker-up:
	docker-compose up -d

# Stop Redis container
docker-down:
	docker-compose down

# Test Ollama connectivity
test-ollama:
	@echo "Testing Ollama connection..."
	@if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then \
		echo "✓ Ollama is running"; \
		echo "Available models:"; \
		curl -s http://localhost:11434/api/tags | grep -o '"name":"[^"]*"' | head -10; \
	else \
		echo "✗ Ollama is not running or not accessible"; \
		exit 1; \
	fi
