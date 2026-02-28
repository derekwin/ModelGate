# ModelGate: 为本地大模型部署打造的 API 网关

## 背景与痛点

随着大语言模型（LLM）的快速发展，越来越多的企业和开发者需要在内部部署和使用本地模型。Ollama、vLLM、llama.cpp 等开源工具让本地模型推理变得简单，但直接将模型服务暴露到公网却面临诸多挑战：

### 安全风险
直接将 Ollama 或 vLLM 暴露在公网，任何人都可以访问你的模型服务，无法控制访问权限，也无法追踪使用情况。

### 多租户管理
企业往往需要为不同用户或部门分配不同的 API Key，设定不同的调用配额和速率限制。直接在模型服务层面实现这些功能既复杂又容易出错。

### 运维复杂性
每个使用方都需要单独管理认证、限流、配额，缺少统一的入口和管理界面，运维成本高昂。

### OpenAI 兼容性
很多应用已经基于 OpenAI API 开发，希望能够零成本迁移到本地模型，或者实现混合部署（部分请求走本地，部分走 OpenAI）。

## 解决方案：ModelGate

ModelGate 是一个专为本地大模型部署设计的 API 网关，用 Go 语言开发，提供 OpenAI 兼容的 API 接口，让你能够安全、受控地向外部或内部用户提供本地模型服务。

### 核心特性

#### 🔐 企业级安全
- API Key 认证：使用 SHA256 哈希存储密钥，安全可靠
- IP 白名单：支持为每个 Key 设定允许访问的 IP 范围
- 传输安全：支持 HTTPS 部署

#### 📊 精细化配额管理
- Token 配额：按需为每个用户分配调用额度
- 速率限制：基于 Redis 的请求频率控制，支持 RPM/Burst
- 使用统计：完整记录每次调用的 Token 消耗

#### 🌐 多后端支持
- Ollama：本地模型推理
- vLLM：高性能推理服务
- llama.cpp：轻量级推理
- OpenAI：与官方 API 混合部署
- API3：第三方 API 集成

#### 📈 零成本迁移
完全兼容 OpenAI API 格式，现有应用只需修改 base_url 即可切换到本地模型：

```python
# 原来的 OpenAI 调用
client = OpenAI(
    api_key="xxx",
    base_url="https://api.openai.com/v1"
)

# 切换到 ModelGate
client = OpenAI(
    api_key="your-modelgate-key",
    base_url="http://your-server:8080/v1"
)
```

#### 🎛 灵活的管理方式
- **Web 管理界面**：可视化操作，无需学习成本
- **CLI 工具**：支持脚本化、自动化运维
- **RESTful API**：深度集成，自定义开发

## 技术架构

### 技术选型

- **Go 1.23+**：高性能、并发友好、部署简单
- **Gin**：轻量级 Web 框架
- **SQLite + GORM**：无需额外数据库依赖
- **Redis**：高性能限流
- **zerolog**：结构化日志

### 架构设计

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│  ModelGate  │────▶│   Ollama   │
│  (OpenAI    │     │  (Gateway)  │     │  / vLLM    │
│   SDK)      │     │             │     │  / llama.cpp
└─────────────┘     └──────┬──────┘     └─────────────┘
                          │
         ┌────────────────┼────────────────┐
         ▼                ▼                ▼
   ┌──────────┐    ┌──────────┐    ┌──────────┐
   │ SQLite   │    │  Redis   │    │  Admin   │
   │ (数据)   │    │ (限流)   │    │  UI/API  │
   └──────────┘    └──────────┘    └──────────┘
```

### 适配器模式

ModelGate 采用适配器模式设计，每种后端（Ollama、vLLM 等）都有对应的适配器，核心逻辑与具体后端解耦，便于扩展新的后端支持。

## 快速开始

### 一键部署

```bash
# Docker Compose 方式（推荐）
docker-compose up -d

# 或手动部署
make build
./modelgate
```

### 配置管理

编辑 `configs/config.yaml`：

```yaml
server:
  port: 8080

admin:
  api_key: "your-admin-key"  # 设置管理员 Key, 可热重载更新

adapters:
  ollama:
    base_url: http://localhost:11434
  vllm:
    base_url: http://localhost:8000
```

### 创建用户 Key

通过 Web 界面或 CLI 创建：

```bash
./modelgate-cli key create -n "user1" -q 1000000 -r 60
```

### 开始使用

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-user-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3:8b",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

## 应用场景

### 场景一：企业内网模型服务

某科技公司有多个人工智能团队需要使用本地部署的 Llama 模型。通过 ModelGate，每个团队拥有独立的 API Key，可以：

- 设置不同的调用配额
- 限制访问 IP 范围（只能从公司内网访问）
- 监控各团队的使用情况

### 场景二：SaaS 平台混合部署

一个 AI 应用平台需要为免费用户提供本地小模型，为付费用户提供 GPT-4。ModelGate 可以实现：

- 免费用户自动路由到本地模型
- 付费用户路由到 OpenAI API
- 统一的计费和监控系统

### 场景三：模型 API 产品化

某团队训练了垂直领域的专业模型，希望对外提供 API 服务。ModelGate 提供了：

- 完整的认证和鉴权
- 按调用量计费的基础
- 可视化管理后台
- API 使用报表

## 与同类产品对比

| 特性 | ModelGate | OpenAI API | Nginx |
|------|-----------|------------|-------|
| 多后端支持 | ✅ | ❌ | ❌ |
| API Key 管理 | ✅ | ✅ | ❌ |
| Token 配额 | ✅ | ✅ | ❌ |
| 速率限制 | ✅ | ✅ | 基础 |
| 使用统计 | ✅ | ✅ | ❌ |
| Web 管理界面 | ✅ | ✅ | ❌ |
| 部署复杂度 | 低 | 无需部署 | 中 |
| 开源 | ✅ | ❌ | ✅ |

## 未来规划

- **插件系统**
- **监控告警**
- **分布式部署**
- **更多后端**

## 结语

ModelGate 致力于解决本地大模型部署的安全和管理难题，让开发者能够专注于模型和应用本身，而不是花大量时间处理认证、限流、运维等基础设施工作。

开源地址：https://github.com/derekwin/ModelGate

欢迎 Star 和贡献！
