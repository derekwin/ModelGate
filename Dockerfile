FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o modelgate cmd/server/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/modelgate .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/admin ./admin

RUN mkdir -p data

EXPOSE 8080

USER app

ENTRYPOINT ["./modelgate"]
