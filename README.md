# ModelGate - LLM API Gateway 完整使用指南

> 生产级 Go 语言 LLM API 网关，支持 Ollama、vLLM、llama.cpp 后端，并提供完整的管理后台界面。

## 目录

- [快速开始](#快速开始)
- [功能特性](#功能特性)
- [构建与安装](#构建与安装)
- [运行方式](#运行方式)
- [配置说明](#配置说明)
- [API 使用](#api-使用)
- [管理后台](#管理后台)
- [自动化脚本](#自动化脚本)
- [常见问题](#常见问题)

---

## 快速开始

### 1. 启动依赖服务

```bash
# 启动 Redis (用于速率限制)
make docker-up

# 停止 Redis
make docker-down
```

### 2. 构建并运行

```bash
# 方式一：使用 Makefile
make build
./bin/modelgate

# 方式二：使用自动化脚本
./run.sh          # 只启动 Gateway
./run.sh admin    # 启动 Gateway + 管理后台
```

### 3. 测试访问

```bash
# 测试 Gateway API
curl -H "Authorization: Bearer test-key" \
  -d '{"model":"gpt-oss:20b","messages":[{"role":"user","content":"你好"}]}' \
  http://localhost:8080/v1/chat/completions

# 访问管理后台 (浏览器)
http://localhost:8080/admin/
```

---

## 功能特性

### Gateway 核心功能

| 功能 | 说明 |
|------|------|
| OpenAI 兼容 API | 完全匹配 OpenAI SDK 接口 |
| 多后端支持 | Ollama、vLLM、llama.cpp |
| API Key 认证 | SHA256 哈希存储 |
| 速率限制 | Redis-based RPM 限制 |
| 配额管理 | Token 配额跟踪 |
| 请求日志 | 完整请求记录 |

### 管理后台功能

| 模块 | 功能 |
|------|------|
| 健康监控 | 实时检查后端 API 状态、响应时间、错误信息 |
| 用户管理 | 创建/编辑/删除多租户用户 |
| API Key 管理 | 生成密钥、设置配额和速率限制 |
| 模型管理 | 配置模型路由、启用/禁用 |
| 用量统计 | 实时监控、趋势图表、API Key 排行 |

---

## 构建与安装

### 前置要求

- Go 1.23+
- Redis (可选，用于速率限制)
- Docker (可选，用于 Redis 容器)

### 编译方式

```bash
# 方式一：Makefile
make build

# 方式二：直接编译
go build -o modelgate ./cmd/server/main.go

# 方式三：使用脚本 (自动编译)
./run.sh build
```

### 目录结构

```
modelgate/
├── cmd/server/main.go          # 主程序入口
├── internal/
│   ├── admin/                   # 管理后台服务
│   │   ├── api.go              # REST API 端点
│   │   ├── health.go           # 健康检查服务
│   │   ├── tenant.go           # 多租户管理
│   │   └── web.go              # 静态文件服务
│   ├── middleware/             # Gin 中间件
│   ├── models/                 # 数据库模型
│   ├── adapters/               # 后端适配器
│   └── service/                # 业务逻辑
├── admin/index.html            # 管理后台前端
├── configs/config.yaml         # 配置文件
├── run.sh                      # 自动化脚本
└── README.md                   # 本文档
```

---

## 运行方式

### 使用自动化脚本 (推荐)

```bash
# 查看帮助
./run.sh help

# 只启动 Gateway (默认)
./run.sh
./run.sh gateway

# 启动 Gateway + 管理后台
./run.sh admin
./run.sh all

# 停止服务
./run.sh stop

# 重启服务
./run.sh restart

# 查看状态
./run.sh status

# 查看日志
./run.sh logs
./run.sh logs gateway

# 重新编译
./run.sh build
```

### 手动运行

```bash
# 直接运行二进制文件
./bin/modelgate

# 后台运行
nohup ./bin/modelgate > logs/gateway.log 2>&1 &

# 停止服务
pkill -f modelgate
```

# 使用指定配置文件
./bin/modelgate -c configs/config.yaml
./bin/modelgate -c configs/production.yaml

---

## 配置说明

### 配置文件

编辑 `configs/config.yaml`:

```yaml
server:
  # 服务监听地址
  host: "0.0.0.0"
  
  # 服务端口
  port: 8080
  
  # 最大请求体大小 (MB)
  max_body_mb: 5
  
  # 请求超时时间 (秒)
  timeout_seconds: 300
  
  # 模型同步间隔 (秒)
  sync_interval: 60

database:
  # SQLite 数据库文件路径
  path: "./data/modelgate.db"

redis:
  # Redis 服务器地址
  addr: "localhost:6379"
  
  # Redis 密码 (如无密码留空)
  password: ""
  
  # Redis 数据库编号
  db: 0

# 后端服务配置
backends:
  - type: "ollama"
    url: "http://localhost:11434"
    model_name: "default"
  
  - type: "vllm"
    url: "http://localhost:8000"
    model_name: "default"

# 日志配置 (可选)
log:
  level: "info"          # debug, info, warn, error
  format: "console"      # console, json
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| MG_SERVER_HOST | 0.0.0.0 | 服务监听地址 |
| MG_SERVER_PORT | 8080 | 服务端口 |
| MG_SERVER_TIMEOUT_SECONDS | 300 | 请求超时 |
| MG_SERVER_MAX_BODY_MB | 5 | 最大请求体 (MB) |
| MG_DATABASE_PATH | ./data/modelgate.db | SQLite 路径 |
| MG_REDIS_ADDR | localhost:6379 | Redis 地址 |
| MG_REDIS_PASSWORD | (空) | Redis 密码 |
| MG_REDIS_DB | 0 | Redis 数据库编号 |

---

## API 使用

### Gateway API (需要认证)

```bash
# Chat Completions
curl -H "Authorization: Bearer test-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-oss:20b",
    "messages": [{"role": "user", "content": "你好"}]
  }' \
  http://192.168.0.107:8080/v1/chat/completions

# 列出模型
curl -H "Authorization: Bearer test-key" \
  http://localhost:8080/v1/models
```

### 管理后台 API (需要认证)

```bash
# 获取所有后端健康状态
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8080/admin/health/all

# 手动触发健康检查
curl -X POST -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8080/admin/health/check

# 获取用户列表
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8080/admin/tenants/users

# 创建用户
curl -X POST -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "test", "email": "test@example.com"}' \
  http://localhost:8080/admin/tenants/users

# 创建 API Key
curl -X POST -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"user_id": 1, "quota_tokens": 1000000, "rate_limit_rpm": 60}' \
  http://localhost:8080/admin/tenants/apikeys

# 获取用量统计
curl -H "Authorization: Bearer YOUR_API_KEY" \
  "http://localhost:8080/admin/usage/stats?start_time=2024-01-01T00:00:00Z&end_time=2024-12-31T23:59:59Z"
```

完整 API 列表见 [API 端点](#api-端点) 章节。

---

## 管理后台

### 访问方式

浏览器打开：`http://localhost:8080/admin/`

### 功能模块

#### 1. 后端健康监控
- 显示所有后端状态 (正常/异常)
- 响应时间监控
- 最后检查时间
- 错误信息展示
- 手动触发检查

#### 2. 多租户管理

**用户管理**
- 创建新用户
- 编辑用户信息
- 删除用户

**API Key 管理**
- 生成新 API Key
- 设置 Token 配额
- 设置 RPM 限制
- 查看使用量

**模型管理**
- 添加模型路由
- 配置后端类型
- 启用/禁用模型

#### 3. 用量统计
- 总消耗 Tokens
- 活跃 API Key 数量
- 用量趋势图表 (ECharts)
- API Key 用量排行
- 最近请求日志

### 技术架构

**后端**
- `internal/admin/api.go` - RESTful API
- `internal/admin/health.go` - 健康检查
- `internal/admin/tenant.go` - 多租户管理
- `internal/admin/web.go` - 静态文件服务

**前端**
- Vue 3 + Element Plus
- ECharts 图表
- Axios HTTP 客户端

---

## 数据库

### 自动迁移表

系统启动时自动创建以下表:

| 表名 | 说明 |
|------|------|
| `users` | 用户表 |
| `api_keys` | API Key 表 |
| `models` | 模型配置表 |
| `backend_health` | 后端健康状态表 |
| `usage_stats` | 用量统计表 |

### 查看数据

```bash
# 使用 SQLite 命令行
sqlite3 ./data/modelgate.db

# 查看表
.tables

# 查询用户
SELECT * FROM users;

# 查询 API Key
SELECT * FROM api_keys;

# 查询用量统计
SELECT * FROM usage_stats ORDER BY timestamp DESC LIMIT 10;
```

---

## 自动化脚本

### run.sh 命令

| 命令 | 说明 |
|------|------|
| `./run.sh` | 启动 Gateway (默认) |
| `./run.sh gateway` | 只启动 Gateway |
| `./run.sh admin` | Gateway + 管理后台 |
| `./run.sh stop` | 停止所有服务 |
| `./run.sh restart` | 重启服务 |
| `./run.sh status` | 查看服务状态 |
| `./run.sh logs` | 查看日志 |
| `./run.sh build` | 重新编译 |
| `./run.sh help` | 显示帮助 |

### 日志管理

```bash
# 日志目录
logs/

# Gateway 日志
logs/gateway.log

# 实时查看日志
./run.sh logs

# 查看特定日志
./run.sh logs gateway
```

---

## API 端点完整列表

### Gateway API

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | /v1/chat/completions | 聊天补全 | ✅ |
| POST | /v1/completions | 文本补全 | ✅ |
| GET | /v1/models | 列出模型 | ✅ |

### 管理后台 API

#### 健康检查
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /admin/health | 获取单个后端健康 |
| GET | /admin/health/all | 获取所有后端健康 |
| POST | /admin/health/check | 手动触发检查 |

#### 多租户管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /admin/tenants/users | 用户列表 |
| POST | /admin/tenants/users | 创建用户 |
| PUT | /admin/tenants/users/:id | 更新用户 |
| DELETE | /admin/tenants/users/:id | 删除用户 |
| GET | /admin/tenants/apikeys | API Key 列表 |
| POST | /admin/tenants/apikeys | 创建 API Key |
| PUT | /admin/tenants/apikeys/:id | 更新 API Key |
| DELETE | /admin/tenants/apikeys/:id | 删除 API Key |
| GET | /admin/tenants/models | 模型列表 |
| POST | /admin/tenants/models | 创建模型 |
| PUT | /admin/tenants/models/:id | 更新模型 |
| DELETE | /admin/tenants/models/:id | 删除模型 |

#### 用量统计
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /admin/usage/stats | 总体统计 |
| GET | /admin/usage/stats/by-time | 按时间统计 |
| GET | /admin/usage/stats/by-key | 按 Key 统计 |
| GET | /admin/usage/logs | 最近日志 |

---

## 常见问题

### Q: Redis 必须吗？
A: 不是必须的。没有 Redis 时速率限制功能会自动禁用。

### Q: 如何创建第一个 API Key？
A: 通过管理后台 API 创建:
```bash
curl -X POST -H "Authorization: Bearer test-key" \
  -H "Content-Type: application/json" \
  -d '{"user_id": 1, "quota_tokens": 1000000, "rate_limit_rpm": 60}' \
  http://localhost:8080/admin/tenants/apikeys
```

### Q: 管理后台无法访问？
A: 检查:
1. 服务是否启动 (`./run.sh status`)
2. 端口是否被占用 (`lsof -i :8080`)
3. 查看日志 (`./run.sh logs`)

### Q: 如何重置数据库？
A: 删除数据库文件后重启:
```bash
rm ./data/modelgate.db
./run.sh restart
```

### Q: 配置文件修改后需要重启吗？
A: 是的，修改 `configs/config.yaml` 后需要重启服务才能生效。

### Q: 生产环境如何部署？
A: 建议:
1. 使用 Docker 容器化部署
2. 配置 HTTPS
3. 修改默认 API Key
4. 启用 Redis 速率限制
5. 配置日志轮转

---

## 测试

```bash
# 运行所有测试
make test

# 运行测试并生成覆盖率报告
make test-cover

# 运行单个测试
go test -v ./internal/admin -run TestHealthCheck
```

---

## 开发者

ModelGate v1.0.0

License: MIT

---

## 更新日志

### v1.0.0
- ✅ Gateway 核心功能
- ✅ 管理后台界面
- ✅ 健康监控系统
- ✅ 多租户管理
- ✅ 用量统计
- ✅ 自动化脚本
- ✅ 配置文件支持
