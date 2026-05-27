# shadraw

Go 后端 for the [shadraw-ui](https://github.com/liusx/shadraw-ui) AI image generation workbench. 前后端分离。

> **第 1 轮（当前）**：项目骨架 + 登录注册 + 工程规范。生图 / 个人数据 / Admin 在后续轮。

## 技术栈

| 维度 | 选型 |
|---|---|
| HTTP | Gin (`github.com/gin-gonic/gin`) |
| ORM | GORM v2 + pgx driver |
| DB | Postgres 16 |
| Migration | `golang-migrate/migrate` |
| Auth | JWT (HS256, access 15min) + refresh (sha256 入库, 7d) + bcrypt |
| Log | 标准库 `log/slog`（JSON handler） |
| Validation | `go-playground/validator/v10`（gin 集成） |

## 目录

```
shadraw/
├── cmd/server/main.go    # 入口、装配
├── internal/
│   ├── app/               # 启动引导（migrations + ensureAdmin）
│   ├── auth/              # JWT / bcrypt / Service / Handler / RequireAuth
│   ├── user/              # User model + Repository
│   ├── config/            # 读 env（必填校验）
│   ├── crypto/            # AES-GCM（占位，后续 admin upstream-config 使用）
│   ├── httpx/             # 响应外壳 / 错误码 / 中间件 / CORS / 限流 / 校验
│   └── store/             # GORM Postgres 连接
├── migrations/            # 001_common, 002_users, 003_refresh_tokens
├── docs/
│   ├── api.md             # 完整接口文档
│   └── db.md              # 表结构与迁移规则
├── data/                  # 本地文件存储挂卷
├── docker-compose.yml
├── Dockerfile
└── Makefile
```

## 本地起步

```bash
cp .env.example .env
# 填上 JWT_SECRET（32+ 字符）、ADMIN_EMAIL、MASTER_KEY（32 字节 base64）
docker compose up --build
# 首次启动会看到一条 ADMIN BOOTSTRAP 日志，里面有 admin 临时密码 — 拷下来后立即登录改密码

curl localhost:8080/healthz
```

仅本机跑后端（DB 通过 docker 起，进程直接 go run）：

```bash
docker compose up -d db
DB_DSN=postgres://shadraw:shadraw@localhost:5432/shadraw?sslmode=disable \
JWT_SECRET=$(openssl rand -hex 32) \
MASTER_KEY=$(openssl rand -base64 32) \
ADMIN_EMAIL=you@example.com \
make run
```

## Env 变量

| Key | 必填 | 说明 |
|---|---|---|
| `PORT` | | 监听端口（默认 8080） |
| `LOG_LEVEL` | | debug/info/warn/error（默认 info） |
| `DB_DSN` | ✅ | Postgres 连接串 |
| `JWT_SECRET` | ✅ | ≥32 字符随机串，HS256 签名密钥 |
| `ADMIN_EMAIL` | ✅ | 首位管理员邮箱，启动时引导 |
| `MASTER_KEY` | ✅ | 32 字节 base64，后续 AES-GCM 加密 apiKey 用 |
| `CORS_ORIGINS` | | 前端 origin 白名单，逗号分隔；默认 `http://localhost:3000` |
| `BLOB_DRIVER` | | 图片存储驱动：`local` 或 `s3`，应用默认 `local`；`.env.example` 使用 `s3` 以配合 docker compose MinIO |
| `DATA_DIR` | | 运行时数据目录，默认 `./data` |
| `S3_ENDPOINT` | `BLOB_DRIVER=s3` 时必填 | S3 兼容 endpoint，例如 `http://localhost:9000` / `http://minio:9000` |
| `S3_REGION` | | S3 region，MinIO 可用默认 `us-east-1` |
| `S3_BUCKET` | `BLOB_DRIVER=s3` 时必填 | 图片对象所在 bucket |
| `S3_ACCESS_KEY_ID` | `BLOB_DRIVER=s3` 时必填 | S3 / MinIO access key |
| `S3_SECRET_ACCESS_KEY` | `BLOB_DRIVER=s3` 时必填 | S3 / MinIO secret key |
| `S3_USE_PATH_STYLE` | | 是否使用 path-style URL，MinIO 默认 `true` |

## 图片存储

默认使用本地文件系统，生成图写入 `DATA_DIR/images/user-<id>/<record-uuid>.<ext>`，数据库只保存相对路径。

如需使用 MinIO / S3 兼容对象存储：

```bash
BLOB_DRIVER=s3
S3_ENDPOINT=http://minio:9000
S3_REGION=us-east-1
S3_BUCKET=shadraw
S3_ACCESS_KEY_ID=shadraw
S3_SECRET_ACCESS_KEY=shadrawsecret
S3_USE_PATH_STYLE=true
```

`docker compose up --build` 会启动 MinIO 和 `minio-init`，自动创建默认 bucket。MinIO 控制台地址是 `http://localhost:9001`。
如果 API 运行在 Docker Compose 内部，`S3_ENDPOINT` 应使用 `http://minio:9000`；如果 API 直接在宿主机运行，则使用 `http://localhost:9000`。

切换存储驱动不会自动迁移旧图片；已有本地图片可以用迁移命令复制到 bucket。迁移会保留数据库里已存的相对路径，例如 `DATA_DIR/images/user-13/a.png` 会上传为对象 key `images/user-13/a.png`，所以不需要改数据库。

```bash
# 先确保 MinIO 已启动。命令在宿主机运行时 endpoint 用 localhost。
S3_ENDPOINT=http://localhost:9000 \
S3_BUCKET=shadraw \
S3_ACCESS_KEY_ID=shadraw \
S3_SECRET_ACCESS_KEY=shadrawsecret \
go run ./cmd/migrate-blobs -data-dir ./data

# 只预览将要上传的文件
go run ./cmd/migrate-blobs -dry-run -data-dir ./data

# 迁移后校验 MinIO 中对象是否存在且大小一致
S3_ENDPOINT=http://localhost:9000 \
S3_BUCKET=shadraw \
S3_ACCESS_KEY_ID=shadraw \
S3_SECRET_ACCESS_KEY=shadrawsecret \
go run ./cmd/migrate-blobs -verify-only -data-dir ./data
```

迁移命令默认读取 `.env` 和 `DATA_DIR=./data`，只上传 `DATA_DIR/images` 下的文件，并跳过 `.DS_Store` 等隐藏文件。它可以重复运行；同名对象会被覆盖。确认迁移成功前不要删除本地 `data/images`。

## 接口（v1）

| Method | Path | 鉴权 | 限流 | 说明 |
|---|---|---|---|---|
| POST | `/api/v1/auth/register` | — | 5/min/IP | 注册并发 token |
| POST | `/api/v1/auth/login` | — | 5/min/IP | 登录并发 token |
| POST | `/api/v1/auth/refresh` | — | 60/min/IP | 刷新 access/refresh 对（rotation） |
| POST | `/api/v1/auth/logout` | Bearer | 60/min/user | 撤销 refresh token |
| GET  | `/api/v1/auth/me` | Bearer | — | 当前用户 |
| POST | `/api/v1/auth/password` | Bearer | 10/min/user | 改密 + 撤销所有 refresh |
| GET  | `/healthz` | — | — | 健康检查 |

详细请求 / 响应 / 错误见 [`docs/api.md`](./docs/api.md)。

## 规范

- 接口规范：统一响应外壳、错误码白名单、状态码语义、ID 字符串化、时间 UTC。
- 数据库规范：snake_case 命名、TIMESTAMPTZ、CITEXT 邮箱、CHECK 替代 ENUM、可逆迁移、不写 seed。
- 测试规范：service mock 单测 + handler 集成测试，auth 模块覆盖率 ≥ 70%。

完整规范见 [`.trellis/tasks/05-25-shadraw-backend-bootstrap/design.md`](https://github.com/liusx/shadraw-ui/tree/main/.trellis/tasks/05-25-shadraw-backend-bootstrap) 的 §10 / §11 / §12。

## 开发命令

```bash
make run          # 本地启动
make test         # 跑测试 + race + cover
make lint         # golangci-lint
make migrate-up   # apply migrations
make migrate-down # rollback 1
make migrate-new NAME=add_xxx  # 新建 migration
```
