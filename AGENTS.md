# ModelGate Development Guidelines

## Project Overview

ModelGate is a production-grade Go-based LLM API Gateway providing OpenAI-compatible endpoints with support for Ollama, vLLM, llama.cpp, OpenAI, and API3 backends. It provides authentication, rate limiting, quota enforcement, and admin panel for key management.

## Core Principles

- Clean Architecture: single responsibility per layer
- Adapter Pattern: backend services decoupled via service.Adapter interface
- OpenAI Compatibility: all external APIs match OpenAI SDK exactly
- HTTP Forwarding: all backends via HTTP, no direct model code coupling
- Security First: SHA256 hashed API keys, 5MB body limits, 300s timeouts, quota enforcement

## Technology Stack

Go 1.23+, Gin (release mode), SQLite+GORM, Redis (go-redis/v8), zerolog, Viper (MG_ prefix), Docker (multi-stage Alpine <60MB)

## Directory Structure

modelgate/
├── cmd/server/main.go          # Entry point
├── internal/
│   ├── adapters/               # Backend adapters
│   ├── admin/                  # Admin API endpoints
│   ├── auth/                   # API key hashing/validation
│   ├── config/                 # Config with hot-reload
│   ├── database/               # GORM setup/migrations
│   ├── limiter/                # Rate limiter logic
│   ├── middleware/             # Auth, rate limit, logging, quota
│   ├── models/                 # Database models
│   ├── service/                # Business logic (GatewayService)
│   ├── usage/                  # Token usage tracking
│   └── utils/                  # Helper functions
├── configs/config.yaml         # Configuration
├── admin/index.html            # Admin panel UI
├── Dockerfile
├── docker-compose.yml
└── Makefile

## Build & Test Commands

make run, build, test, test-cover, docker-up, docker-down, test-ollama

### Run Single Tests

go test -v ./internal/{auth,middleware,limiter,utils,config,adapters} -run {TestHashAPIKey,TestAuthMiddleware,TestRateLimiter,TestErrorResponse,TestLoadConfig,TestOllamaAdapter}

go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

## Code Style

- Use gofmt for formatting
- Imports: stdlib, external, internal
- File naming: snake_case.go
- Function names: PascalCase for public, camelCase for private
- Error handling: always check errors, wrap with %w

### Types & Interfaces

type Adapter interface {
    ChatCompletion(ctx context.Context, req OpenAIRequest, model Model) (*OpenAIResponse, error)
    Completion(ctx context.Context, req OpenAIRequest, model Model) (*OpenAIResponse, error)
    Models(ctx context.Context, model Model) (*OpenAIModelsResponse, error)
}

### Error Handling

return nil, fmt.Errorf("failed: %w", err)
return nil, &APIError{Message: "..", Type: "..", Code: 401}

### Logging

Use zerolog with structured fields: request ID, user ID, model name

log.Debug().Str("model", name).Int("user_id", id).Msg("Processing")

## API Design

All request/response bodies must match OpenAI API spec.

### HTTP Methods

POST /v1/chat/completions, POST /v1/completions, GET /v1/models

### Error Response

{"error": {"message": "..", "type": "..", "code": 401}}

## Middleware

Auth: Bearer token, Rate Limiter: Redis INCR+EXPIRE (RPM), Quota Check: token quota, Logging: method, path, IP, user ID, duration, Body Size: 5MB limit

## Database

All models embed BaseModel:

type BaseModel struct {
    ID uint gorm:"primarykey"
    CreatedAt time.Time
    UpdatedAt time.Time
    Status string
}

Use GORM auto-migration for development.

## Adapter Pattern

Route based on Model.BackendType: Ollama, vLLM, llama.cpp, OpenAI, API3

All adapters handle HTTP errors and convert to OpenAI format.

## Testing

Unit: test each layer, mock dependencies. Integration: full request flow. Coverage: 80%+

go test -v ./... -run TestAuthMiddleware
go test -v ./internal/adapters -count=1
make test-cover

## Security

Never log API keys (even hashed), never expose backend URLs, validate user input, 300s timeout, 5MB body limit, use context.WithTimeout

## Docker

FROM golang:1.23-alpine AS builder
FROM alpine:latest
RUN adduser -D app
USER app
COPY modelgate .
EXPOSE 8080
ENTRYPOINT ["./modelgate"]

## Default Configuration

| Setting | Default |
|Port|8080|
|DB Path|./data/modelgate.db|
|Redis Addr|localhost:6379|
|Timeout|300s|
|Max Body|5 MB|
|Rate Limit|60 RPM|

## Common Pitfalls

1. Always use %w for error wrapping
2. Pass context through all layers
3. Close DB connections on shutdown
4. Use shared HTTP client with timeout
5. Use DB transactions for quota updates
6. Rate limiter uses atomic operations
7. Validate model exists before routing

## Client

Admin API provides key management endpoints (create, list, update, delete), all require authentication, admin panel at /admin/ for self-service, token quota and rate limits enforced per API key

## Cursor Rules / Copilot Instructions

Standard Go conventions and TypeScript/JavaScript patterns for admin UI. No specific rules defined.
