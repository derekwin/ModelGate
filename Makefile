.PHONY: build run test test-cover docker-up docker-down docker-build clean build-cli

BINARY_NAME=modelgate
CLI_NAME=modelgate-cli
MAIN_PATH=cmd/server/main.go

all: build build-cli

build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

build-cli:
	go build -o $(CLI_NAME) ./cmd/cli/main.go

run: build
	lsof -ti :18080 | xargs -r kill -9
	./$(BINARY_NAME)

test:
	go test -v ./...

test-cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

docker-build:
	docker build -t modelgate:latest .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

test-ollama:
	@echo "Testing Ollama adapter..."
	@echo "Make sure Ollama is running on localhost:11434"

clean:
	rm -f $(BINARY_NAME) coverage.out
	rm -rf data/*.db

lint:
	gofmt -w .
	go vet ./...

install-deps:
	go mod download
	go mod tidy
