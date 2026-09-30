<p align="center">
  <img src="assets/banner.svg" alt="Codex2API" width="100%">
</p>

<p align="center">
  <a href="README.md">English</a> | <b>中文</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/Gin-1.12-00ACD7?style=for-the-badge" alt="Gin">
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=for-the-badge&logo=react&logoColor=111827" alt="React">
  <img src="https://img.shields.io/badge/Vite-8-646CFF?style=for-the-badge&logo=vite&logoColor=white" alt="Vite">
  <img src="https://img.shields.io/badge/DB-PostgreSQL%20%7C%20SQLite-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="Database">
  <img src="https://img.shields.io/badge/Cache-Redis%20%7C%20Memory-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Cache">
  <img src="https://img.shields.io/badge/API-OpenAI%20%7C%20Anthropic-10A37F?style=for-the-badge" alt="API">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker">
</p>

**Codex2API 把 Codex 账号池变成可观测、可调度的 OpenAI / Anthropic 兼容网关。** 它提供 Chat Completions、Responses、Messages、Images、Models 和管理接口，并负责账号调度、Token 刷新、健康状态、限流恢复和用量记录。

本仓库是在上游项目基础上的持续维护版本，增加了计费、上游状态监控和移动端用量展示能力。上游功能继续保留，除非与下面列出的自定义行为发生冲突。

## 自定义维护内容

- **上游倍率探查：** 接入 Sub2API 的账号可以选择开启倍率探查，支持手动探查和定时探查，并对响应协议进行严格校验。临时探查失败时保留最后一次有效倍率。
- **倍率计费：** 普通账号按照官方模型价格计算。探查到有效上游倍率后，用户费用和上游成本估算以官方基础价格为依据，再单独应用倍率。账号、API Key 和使用统计页面会显示倍率及对应费用信息。
- **渠道健康监控：** 管理后台可以主动检查渠道可用性、探查状态、响应时间和近期失败记录，不需要等到用户请求发生后才发现问题。
- **移动端用量展示：** 费用明细、提示信息和账号信息在小屏幕上保持清晰可读，移动端点击提示也能正常使用。
- **模型价格覆盖：** 维护版本补齐当前上游模型别名、标准价格、长上下文边界，以及上游价格探查不可用时的回退路径。

## 快速部署

完整部署说明请参考：[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)

### 部署模式

| 模式 | 文件 | 适用场景 |
| --- | --- | --- |
| Docker 镜像 | <code>docker-compose.yml</code> | 服务器和测试环境，推荐使用 |
| 本地源码构建 | <code>docker-compose.local.yml</code> | 构建并验证当前源码 |
| SQLite 镜像 | <code>docker-compose.sqlite.yml</code> | 不依赖 PostgreSQL / Redis 的单机部署 |
| SQLite 源码构建 | <code>docker-compose.sqlite.local.yml</code> | 验证轻量 SQLite 模式 |
| 本地开发 | <code>go run .</code> + <code>npm run dev</code> | 前后端开发调试 |

### 标准部署

~~~bash
git clone --branch codex2api-custom https://github.com/JayHome137/codex2api.git
cd codex2api
cp .env.example .env
docker compose pull
docker compose up -d
docker compose logs -f codex2api
~~~

### 本地源码构建

~~~bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
docker compose -f docker-compose.local.yml logs -f codex2api
~~~

### SQLite 部署

~~~bash
cp .env.sqlite.example .env
docker compose -f docker-compose.sqlite.yml pull
docker compose -f docker-compose.sqlite.yml up -d
docker compose -f docker-compose.sqlite.yml logs -f codex2api
~~~

SQLite compose 文件默认绑定 <code>127.0.0.1</code>。需要外部访问时，在 <code>.env</code> 中设置 <code>BIND_HOST=0.0.0.0</code>。标准 compose 文件默认监听所有网络接口。

启动后访问：

- 管理后台：<code>http://localhost:8080/admin/</code>
- 健康检查：<code>http://localhost:8080/health</code>

<code>docker compose down</code> 默认会保留命名卷。只有在明确需要删除持久化数据时，才使用 <code>docker compose down -v</code>。

## 升级与本地开发

升级正在运行的镜像部署：

~~~bash
git pull
docker compose pull
docker compose up -d
~~~

升级前备份 PostgreSQL：

~~~bash
docker exec codex2api-postgres pg_dump -U codex2api codex2api > backup_$(date +%Y%m%d_%H%M%S).sql
~~~

首次运行后端前，需要先构建前端，因为 Go 会通过 <code>go:embed</code> 嵌入 <code>frontend/dist</code>：

~~~bash
cp .env.example .env
cd frontend && npm ci && npm run build && cd ..
go run .
~~~

前端开发服务器：

~~~bash
cd frontend && npm ci && npm run dev
~~~

前端开发时访问 <code>http://localhost:5173/admin/</code>。

## 环境配置

标准 <code>.env.example</code> 使用 PostgreSQL 和 Redis，SQLite 模式使用 <code>.env.sqlite.example</code>。

| 变量 | 说明 |
| --- | --- |
| <code>CODEX_PORT</code> | HTTP 端口，默认 <code>8080</code> |
| <code>BIND_HOST</code> | 监听地址，例如 <code>127.0.0.1</code> 或 <code>0.0.0.0</code> |
| <code>ADMIN_SECRET</code> | 管理后台登录密钥 |
| <code>DATABASE_DRIVER</code> | <code>postgres</code> 或 <code>sqlite</code> |
| <code>DATABASE_PATH</code> | <code>DATABASE_DRIVER=sqlite</code> 时的数据库路径 |
| <code>DATABASE_HOST</code> / <code>DATABASE_PORT</code> | PostgreSQL 连接地址 |
| <code>DATABASE_USER</code> / <code>DATABASE_PASSWORD</code> / <code>DATABASE_NAME</code> | PostgreSQL 凭据和数据库名称 |
| <code>CACHE_DRIVER</code> | <code>redis</code> 或 <code>memory</code> |
| <code>REDIS_ADDR</code> | Redis 地址或 URL |
| <code>TZ</code> | IANA 时区，例如 <code>Asia/Shanghai</code> |

调度模式、请求限制和计费选项等业务设置保存在数据库中，可从管理后台修改。完整配置参考：[docs/CONFIGURATION.md](docs/CONFIGURATION.md)

## API 与管理后台

| 接口 | 说明 |
| --- | --- |
| <code>POST /v1/chat/completions</code> | OpenAI 兼容 Chat Completions |
| <code>POST /v1/responses</code> | Responses API |
| <code>POST /v1/messages</code> | Anthropic Messages API |
| <code>POST /v1/images/generations</code> | 图片生成 |
| <code>GET /v1/models</code> | 可用模型列表 |
| <code>GET /health</code> | 健康检查 |

主要管理页面包括 <code>/admin/accounts</code>、<code>/admin/api-keys</code>、<code>/admin/usage</code>、<code>/admin/channel-monitors</code>、<code>/admin/settings</code> 和 <code>/admin/ops</code>。API Key 和管理密钥都在管理后台配置。

费用统计使用模型价格表和[自定义维护内容](#自定义维护内容)中的倍率状态。倍率探查失败或没有可用倍率时，回退到官方模型价格。

## 文档

- [部署说明](docs/DEPLOYMENT.md)
- [使用指南](docs/USAGE.md)
- [API 参考](docs/API.md)
- [配置说明](docs/CONFIGURATION.md)
- [架构说明](docs/ARCHITECTURE.md)
- [故障排查](docs/TROUBLESHOOTING.md)
- [English README](README.md)

## 免责声明与协议

本项目用于学习、研究和技术交流。请只在你有权访问的上游服务中使用，并自行承担部署和使用责任。本项目采用 MIT 协议发布，不提供任何明示或默示担保。
