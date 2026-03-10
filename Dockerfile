FROM golang:1.22-bookworm AS builder

RUN if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
        sed -i 's|http://deb.debian.org/debian|http://mirrors.tuna.tsinghua.edu.cn/debian|g; s|http://security.debian.org/debian-security|http://mirrors.tuna.tsinghua.edu.cn/debian-security|g' /etc/apt/sources.list.d/debian.sources; \
    fi \
    && if [ -f /etc/apt/sources.list ]; then \
        sed -i 's|http://deb.debian.org/debian|http://mirrors.tuna.tsinghua.edu.cn/debian|g; s|http://security.debian.org/debian-security|http://mirrors.tuna.tsinghua.edu.cn/debian-security|g' /etc/apt/sources.list; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends build-essential ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o modelgate cmd/server/main.go

FROM debian:bookworm-slim

RUN if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
        sed -i 's|http://deb.debian.org/debian|http://mirrors.tuna.tsinghua.edu.cn/debian|g; s|http://security.debian.org/debian-security|http://mirrors.tuna.tsinghua.edu.cn/debian-security|g' /etc/apt/sources.list.d/debian.sources; \
    fi \
    && if [ -f /etc/apt/sources.list ]; then \
        sed -i 's|http://deb.debian.org/debian|http://mirrors.tuna.tsinghua.edu.cn/debian|g; s|http://security.debian.org/debian-security|http://mirrors.tuna.tsinghua.edu.cn/debian-security|g' /etc/apt/sources.list; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN useradd -m -d /app app

WORKDIR /app

COPY --from=builder /app/modelgate .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/admin ./admin

RUN mkdir -p data \
    && chown -R app:app /app

EXPOSE 8080 8081

USER app

ENTRYPOINT ["./modelgate"]
