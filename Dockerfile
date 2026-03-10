FROM golang:1.22-alpine AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o modelgate cmd/server/main.go

FROM alpine:latest

RUN sed -i 's|https://dl-cdn.alpinelinux.org/alpine|https://mirrors.tuna.tsinghua.edu.cn/alpine|g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates
RUN adduser -D -h /app app

WORKDIR /app

COPY --from=builder /app/modelgate .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/admin ./admin

RUN mkdir -p data \
    && chown -R app:app /app

EXPOSE 18080

USER app

ENTRYPOINT ["./modelgate"]
