<p align="center">
  <img src="assets/banner.svg" alt="Codex2API" width="100%">
</p>

<p align="center">
  <b>English</b> | <a href="README.zh-CN.md">中文</a>
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

**Codex2API turns a Codex account pool into an observable, schedulable OpenAI / Anthropic compatible gateway.** It provides Chat Completions, Responses, Messages, Images, Models and administration endpoints while handling account selection, token refresh, health state, rate-limit recovery and usage records.

This repository is a maintained fork with additional billing, upstream monitoring and responsive usage features. Upstream functionality remains available unless it conflicts with the maintained custom behavior described below.

## Custom maintenance

- **Upstream multiplier discovery:** Accounts connected to Sub2API can opt into multiplier discovery. Manual probing and periodic probing are supported, with strict response validation and the last valid value retained for temporary probe failures.
- **Multiplier-aware billing:** Normal accounts use the official model price. When a valid upstream multiplier is available, user billing and upstream cost estimation keep the official base price and apply the multiplier separately. The account, API key and usage views expose the multiplier and the related cost details.
- **Channel health monitoring:** Channel availability, probe status, response time and recent failures can be checked from the administration console without waiting for a user request.
- **Mobile usage details:** Usage cost details, tooltips and account information remain readable and usable on small screens as well as desktop screens.
- **Pricing coverage:** The maintained pricing mapping covers current upstream model aliases, standard and long-context boundaries, and the fallback path used when a model is not returned by an upstream pricing probe.

## Quick start

For the full deployment guide, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

### Deployment modes

| Mode | File | Use case |
| --- | --- | --- |
| Docker image | <code>docker-compose.yml</code> | Recommended for servers and test environments |
| Local source build | <code>docker-compose.local.yml</code> | Build and verify the current source |
| SQLite image | <code>docker-compose.sqlite.yml</code> | Single-node deployment without PostgreSQL or Redis |
| SQLite source build | <code>docker-compose.sqlite.local.yml</code> | Verify the lightweight SQLite mode |
| Local development | <code>go run .</code> + <code>npm run dev</code> | Backend and frontend development |

### Standard deployment

~~~bash
git clone --branch codex2api-custom https://github.com/JayHome137/codex2api.git
cd codex2api
cp .env.example .env
docker compose pull
docker compose up -d
docker compose logs -f codex2api
~~~

### Local source build

~~~bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
docker compose -f docker-compose.local.yml logs -f codex2api
~~~

### SQLite deployment

~~~bash
cp .env.sqlite.example .env
docker compose -f docker-compose.sqlite.yml pull
docker compose -f docker-compose.sqlite.yml up -d
docker compose -f docker-compose.sqlite.yml logs -f codex2api
~~~

The SQLite compose files bind to <code>127.0.0.1</code> by default. Set <code>BIND_HOST=0.0.0.0</code> when external access is required. The standard compose files bind to all interfaces by default.

After startup:

- Admin dashboard: <code>http://localhost:8080/admin/</code>
- Health check: <code>http://localhost:8080/health</code>

Named volumes are preserved by <code>docker compose down</code>. Use <code>docker compose down -v</code> only when you intentionally want to remove persisted data.

## Upgrade and local development

Upgrade a running image deployment:

~~~bash
git pull
docker compose pull
docker compose up -d
~~~

Back up PostgreSQL before an upgrade:

~~~bash
docker exec codex2api-postgres pg_dump -U codex2api codex2api > backup_$(date +%Y%m%d_%H%M%S).sql
~~~

The frontend must be built before the first backend run because Go embeds <code>frontend/dist</code>:

~~~bash
cp .env.example .env
cd frontend && npm ci && npm run build && cd ..
go run .
~~~

For frontend development:

~~~bash
cd frontend && npm ci && npm run dev
~~~

Open <code>http://localhost:5173/admin/</code> during frontend development.

## Configuration

The standard <code>.env.example</code> uses PostgreSQL and Redis. The SQLite mode uses <code>.env.sqlite.example</code>.

| Variable | Description |
| --- | --- |
| <code>CODEX_PORT</code> | HTTP port, default <code>8080</code> |
| <code>BIND_HOST</code> | Listen address, for example <code>127.0.0.1</code> or <code>0.0.0.0</code> |
| <code>ADMIN_SECRET</code> | Admin dashboard login secret |
| <code>DATABASE_DRIVER</code> | <code>postgres</code> or <code>sqlite</code> |
| <code>DATABASE_PATH</code> | SQLite database path when <code>DATABASE_DRIVER=sqlite</code> |
| <code>DATABASE_HOST</code> / <code>DATABASE_PORT</code> | PostgreSQL connection address |
| <code>DATABASE_USER</code> / <code>DATABASE_PASSWORD</code> / <code>DATABASE_NAME</code> | PostgreSQL credentials and database |
| <code>CACHE_DRIVER</code> | <code>redis</code> or <code>memory</code> |
| <code>REDIS_ADDR</code> | Redis address or URL |
| <code>TZ</code> | IANA timezone, for example <code>Asia/Shanghai</code> |

Business settings such as scheduler mode, request limits and billing options are stored in the database and managed from the admin console. See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the complete reference.

## API and administration

| Endpoint | Description |
| --- | --- |
| <code>POST /v1/chat/completions</code> | OpenAI-compatible Chat Completions |
| <code>POST /v1/responses</code> | Responses API |
| <code>POST /v1/messages</code> | Anthropic Messages API |
| <code>POST /v1/images/generations</code> | Image generation |
| <code>GET /v1/models</code> | Available models |
| <code>GET /health</code> | Health check |

The main administration pages are <code>/admin/accounts</code>, <code>/admin/api-keys</code>, <code>/admin/usage</code>, <code>/admin/channel-monitors</code>, <code>/admin/settings</code> and <code>/admin/ops</code>. Public API keys and the admin secret are configured from the administration console.

Pricing uses the model pricing table and the custom multiplier state described in [Custom maintenance](#custom-maintenance). A failed or unavailable multiplier probe falls back to the official model price.

## Documentation

- [Deployment](docs/DEPLOYMENT.md)
- [Usage](docs/USAGE.md)
- [API reference](docs/API.md)
- [Configuration](docs/CONFIGURATION.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)
- [中文说明](README.zh-CN.md)

## Disclaimer and license

This project is provided for learning, research and technical discussion. Use it only where you have the right to access the upstream services and accept responsibility for your deployment. The project is released under the MIT License without warranty.
