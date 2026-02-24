# ModelGate Development Guidelines

## Project Overview

ModelGate is a production-grade Go-based LLM API Gateway that provides OpenAI-compatible endpoints while supporting multiple backend inference services (Ollama, vLLM, llama.cpp). It acts as a reverse proxy with authentication, rate limiting, and usage tracking.

## Core Principles

- **Clean Architecture**: Separate concerns into distinct layers (config, middleware, service, adapters, models)
- **Adapter Pattern**: Backend services are decoupled via adapter interfaces
- **OpenAI Compatibility**: All external APIs must match OpenAI SDK structure exactly
- **HTTP Forwarding Only**: No direct model code coupling; all backends accessed via HTTP
- **Security First**: API keys hashed with SHA256, request body limits, timeouts, quota enforcement

## Technology Stack

- **Language**: Go 1.23+
- **Web Framework**: Gin
- **Database**: SQLite with GORM
- **Redis**: Rate limiting with go-redis
- **Logging**: zerolog
- **Config**: Viper
- **Container**: Docker (multi-stage, Alpine-based, <60MB)

## Directory Structure

```
modelgate/
├── cmd/server/main.go          # Application entry point
├── internal/
│   ├── config/                 # Configuration management
│   ├── database/               # GORM setup and migrations
│   ├── models/                 # Database models (User, APIKey, Model)
│   ├── middleware/             # Gin middleware (auth, rate limit, logging)
│   ├── limiter/                # Redis rate limiter logic
│   ├── auth/                   # API key validation
│   ├── router/                 # Route setup and model registry
│   ├── adapters/               # Backend adapters (base, ollama, vllm, llamacpp)
│   ├── service/                # Business logic layer
│   ├── usage/                  # Token usage tracking
│   └── utils/                  # Helper functions
├── configs/config.yaml         # Configuration file
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── README.md
```

## Build & Test Commands

```bash
make run              # Start server
make build            # Build binary
make test             # Run all tests
make test-cover       # Run tests with coverage report

make docker-build     # Build Docker image
make docker-up        # Start services
make docker-down      # Stop services
```

### Running Single Tests

```bash
go test -v ./internal/middleware -run TestRateLimiter
go test -v ./internal/adapters -run TestOllamaAdapter
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Code Style Guidelines

### Basics

- Use `gofmt` for formatting
- Imports sorted: stdlib, external, internal
- File naming: `snake_case.go`
- Function names: `PascalCase` for public, `camelCase` for private
- Error handling: Always check errors, wrap with `%w`

### Imports

```go
import (
    "context"
    "net/http"
    "github.com/gin-gonic/gin"
    "modelgate/internal/config"
)
```

### Types & Interfaces

```go
type Adapter interface {
    ChatCompletion(ctx context.Context, req OpenAIRequest, model Model) (*OpenAIResponse, error)
    Completion(ctx context.Context, req OpenAIRequest, model Model) (*OpenAIResponse, error)
    Models(ctx context.Context, model Model) (*OpenAIModelsResponse, error)
}
```

### Error Handling

```go
return nil, fmt.Errorf("failed: %w", err)
return nil, &APIError{Message: "...", Type: "...", Code: 401}
```

### Logging

- Use zerolog with structured fields
- Include: request ID, user ID, model name

```go
log.Debug().Str("model", name).Int("user_id", id).Msg("Processing")
```

### Configuration

- Use Viper for config loading
- ENV variables override config file with MG_ prefix

## API Design

All request/response bodies must match OpenAI API spec.

### HTTP Methods

- `POST /v1/chat/completions` - Chat completions
- `POST /v1/completions` - Completions  
- `GET /v1/models` - List models

### Error Response

```json
{"error": {"message": "...", "type": "...", "code": 401}}
```

## Middleware

- **Auth**: Extract/validate Bearer token from `Authorization`
- **Rate Limiter**: Redis INCR+EXPIRE per API key (RPM)
- **Quota Check**: Verify sufficient token quota
- **Logging**: Log method, path, IP, user ID, duration
- **Body Size**: Enforce 5MB limit

## Database

All models embed BaseModel:

```go
type BaseModel struct {
    ID        uint      `gorm:"primarykey"`
    CreatedAt time.Time
    UpdatedAt time.Time
    Status    string
}
```

- Use GORM auto-migration for development

## Adapter Pattern

Route based on `Model.BackendType`:

- **Ollama**: `/v1/chat/completions` + `/api/generate` fallback
- **vLLM**: Direct `/v1/chat/completions` forwarding
- **llama.cpp**: `/v1/chat/completions` or `/completion` fallback

All adapters handle HTTP errors and convert to OpenAI format.

## Testing

- Unit: test each layer independently, mock dependencies
- Integration: full request flow, test containers
- Coverage: 80%+

```bash
go test -v ./... -run TestAuthMiddleware
go test -v ./internal/adapters -count=1
make test-cover
```

## Security

- Never log API keys (even hashed)
- Never expose backend URLs to clients
- Validate user input
- Request timeout: default 300s
- Body limit: 5MB
- Use `context.WithTimeout` for all backend calls

## Docker

```dockerfile
FROM golang:1.23-alpine AS builder
# Build stage

FROM alpine:latest
RUN adduser -D app
USER app
COPY modelgate .
EXPOSE 8080
ENTRYPOINT ["./modelgate"]
```

## Default Configuration

| Setting | Default |
|---------|-----|
| Port | 8080 |
| DB Path | ./data/modelgate.db |
| Redis Addr | localhost:6379 |
| Timeout | 300s |
| Max Body | 5 MB |
| Rate Limit | 60 RPM |

## Common Pitfalls

1. Always use `%w` for error wrapping
2. Pass context through all layers
3. Close DB connections on shutdown
4. Use shared HTTP client with timeout
5. Use DB transactions for quota updates
6. Rate limiter uses atomic operations
7. Validate model exists before routing
