# ModelGate：本地大模型 API 网关解决方案

## 背景与痛点

随着大语言模型（LLM）的快速发展，越来越多的开发者需要在内部部署和使用本地模型。Ollama、vLLM、llama.cpp 等开源工具让本地模型推理变得简单，但直接将模型服务暴露却面临诸多挑战：

### 🔐 安全风险

- 直接将 Ollama 或 vLLM 暴露在公网，任何人都可以访问你的模型服务  
- 无法控制访问权限  
- 无法追踪使用情况  

### 👥 多租户管理

- 需要为不同用户或应用分配不同的 API Key  
- 需要设定不同的调用配额和速率限制  
- 在模型服务层面直接实现这些功能复杂且容易出错  

### ⚙️ 运维复杂性

- 每个使用方都需要单独管理认证、限流、配额  
- 缺少统一入口和管理界面  
- 运维成本高昂  

### 🔄 OpenAI 兼容性

- 很多应用已基于 OpenAI API 开发  
- 希望零成本迁移到本地模型  
- 或实现混合部署（部分请求走本地，部分走 OpenAI）

---

## 解决方案：ModelGate

**ModelGate** 是一个专为本地大模型部署设计的 API 网关，使用 Go 语言开发，提供 OpenAI 兼容的 API 接口，让你能够安全、受控地向外部或内部用户提供本地模型服务。


## 核心特性

### 🔐 安全

- API Key 认证：使用 SHA256 哈希存储密钥  
- IP 白名单：支持为每个 Key 设定允许访问的 IP 范围  
- 传输安全：支持 HTTPS 部署  

### 📊 精细化配额管理

- Token 配额：为每个用户分配调用额度  
- 速率限制：基于 Redis 的请求频率控制，支持 RPM / Burst  
- 使用统计：完整记录每次调用的 Token 消耗  

### 🌐 多后端支持

- Ollama：本地模型推理  
- vLLM：高性能推理服务  
- llama.cpp：轻量级推理  
- OpenAI：与官方 API 混合部署  
- API3：第三方 API 集成  

### 📈 零成本迁移

完全兼容 OpenAI API 格式，现有应用只需修改 `base_url` 即可切换到本地模型：

```python
# 原来的 OpenAI 调用
client = OpenAI(
    api_key="xxx",
    base_url="https://api.openai.com/v1"
)

# 切换到 ModelGate
client = OpenAI(
    api_key="your-modelgate-key",
    base_url="http://your-server:18080/v1"
)
````

### 🎛 灵活的管理方式

* Web 管理界面：可视化操作
* CLI 工具：支持脚本化、自动化运维
* RESTful API：支持深度集成与自定义开发


## 技术架构

### 架构设计

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│  ModelGate  │────▶│   Ollama    │
│  (OpenAI    │     │  (Gateway)  │     │  / vLLM     │
│   SDK)      │     │             │     │  / llama.cpp│
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

ModelGate 采用**适配器模式**设计，每种后端（Ollama、vLLM 等）都有对应适配器，核心逻辑与具体后端解耦，便于扩展新的后端支持。


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
  port: 18080

admin:
  api_key: "your-admin-key"  # 管理员 Key，支持热重载

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
curl -X POST http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer your-user-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3:8b",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```


# 应用场景

## 场景一：企业内网模型服务治理

### 背景

多个 AI 团队需要调用本地部署的 Llama 模型，需要统一治理。

### 通过 ModelGate 可实现

* 🔐 独立 API Key 精细化权限管理
* 📊 设置不同调用配额（QPS / 日上限 / Token 限额）
* 🌐 IP 范围限制（仅允许内网访问）
* 📈 实时监控（调用量 / 失败率 / 响应时间）
* 🧾 用量统计与成本分摊

### 价值

实现企业级模型统一接入与安全管控，提高资源利用率。


## 场景二：SaaS 平台混合模型部署

### 背景

构建分层服务体系：

* 免费用户 → 本地模型
* 付费用户 → GPT-4 等云端模型

### 通过 ModelGate 可实现

* 🔄 基于用户身份自动路由
* ⚖️ 多模型负载均衡
* 💰 统一计费体系
* 📊 统一监控与日志管理
* 🔌 对上层业务提供一致 API

### 价值

实现“混合云 + 分级服务”架构，降低成本同时提升体验。


## 场景三：模型 API 产品化输出

### 背景

将垂直领域模型（医疗 / 法律 / 金融）对外提供为 API 产品。

### 通过 ModelGate 可实现

* 🔐 完整认证与鉴权体系
* 📦 OpenAI 协议兼容封装
* 💳 按调用量 / Token / 套餐计费
* 📊 使用报表与行为分析
* 🖥 可视化管理后台
* 🚦 流量控制与限速保护

### 价值

帮助团队快速将模型能力产品化，构建可运营、可计费、可扩展的模型服务平台。


## 场景四：本地 OpenClaw 多实例调度与负载均衡

### 背景

研究多 OpenClaw 实例协同行为，希望独立配额与负载调度。

### 通过 ModelGate 可实现

* 🦞 为每个实例分配独立 API Key
* ⚖️ 多实例负载均衡
* 📊 实时资源监控
* 🔄 策略路由
* 🚧 单实例调用上限保护
* 📈 行为数据分析

### 价值

构建本地多模型协同实验环境，实现资源可控、行为可观测。


# 优势对比

| 特性         | ModelGate | OpenAI API | Nginx |
| ---------- | --------- | ---------- | ----- |
| 多后端支持      | ✅         | ❌          | ❌     |
| API Key 管理 | ✅         | ✅          | ❌     |
| Token 配额   | ✅         | ✅          | ❌     |
| 速率限制       | ✅         | ✅          | 基础    |
| 使用统计       | ✅         | ✅          | ❌     |
| Web 管理界面   | ✅         | ✅          | ❌     |
| 部署复杂度      | 低         | 无需部署       | 中     |
| 开源         | ✅         | ❌          | ✅     |


# 未来规划

* 插件系统
* 监控告警
* 分布式部署
* 更多后端支持


# 结语

ModelGate 致力于解决本地大模型部署的安全与管理难题，让开发者专注于模型与应用本身，而不是处理认证、限流、运维等基础设施问题。

**开源地址：**
[https://github.com/derekwin/ModelGate](https://github.com/derekwin/ModelGate)

欢迎 Star 和贡献！
