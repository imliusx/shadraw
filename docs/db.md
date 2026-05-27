# shadraw 数据库 schema

> 当前轮（auth 阶段）建立的表。完整业务表（records / projects / upstream_configs）见 `.trellis/tasks/05-25-shadraw-backend-bootstrap/design.md §3`。

## 命名约定

- 表名 `snake_case` 复数
- 列名 `snake_case`
- 外键：`<referenced_singular>_id`
- 索引：`idx_<table>_<col>`；唯一索引 `uq_<table>_<col>`
- 触发器：`trg_<table>_<purpose>`
- 时间一律 `TIMESTAMPTZ`，字符串一律 `TEXT`，邮箱用 `CITEXT`

## 通用：001_common

```sql
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

## 002_users

```sql
CREATE TABLE users (
    id                   BIGSERIAL PRIMARY KEY,
    email                CITEXT NOT NULL,
    password_hash        TEXT NOT NULL,
    display_name         TEXT NOT NULL,
    role                 TEXT NOT NULL DEFAULT 'user'
                         CHECK (role IN ('admin','user')),
    disabled             BOOLEAN NOT NULL DEFAULT FALSE,
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_users_email ON users (email);

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

| 列 | 说明 |
|---|---|
| `email` | CITEXT 使邮箱大小写不敏感地唯一 |
| `password_hash` | bcrypt cost=12 输出 |
| `role` | CHECK 约束限定 admin/user 两值，避免 PG ENUM |
| `disabled` | admin 后台禁用时置 true，登录立即 403 |
| `must_change_password` | admin 重置密码后置 true（本轮仅在 admin 引导时使用） |

## 003_refresh_tokens

```sql
CREATE TABLE refresh_tokens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_refresh_tokens_token_hash ON refresh_tokens (token_hash);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

CREATE TRIGGER trg_refresh_tokens_updated_at
BEFORE UPDATE ON refresh_tokens
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

| 列 | 说明 |
|---|---|
| `token_hash` | 持久化的是 `sha256(rawRefreshToken)` 的十六进制串，原值不入库 |
| `expires_at` | 默认 issue 时 + 7 天（`auth.RefreshTTL`） |
| `revoked` | 主动 logout、refresh rotation、change password 后置 true |

## 011_record_visibility

```sql
ALTER TABLE records
    ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN published_at TIMESTAMPTZ;

CREATE INDEX idx_records_public_gallery
ON records (published_at DESC, id DESC)
WHERE is_public = TRUE AND status = 'completed' AND image_path IS NOT NULL;
```

| 列 | 说明 |
|---|---|
| `is_public` | 用户是否发布到社区画廊；默认 false，生成图片默认私密 |
| `published_at` | 发布到社区画廊的时间；取消公开时清空 |

## 012_record_prompt_visibility

```sql
ALTER TABLE records
    ADD COLUMN prompt_public BOOLEAN NOT NULL DEFAULT TRUE;
```

| 列 | 说明 |
|---|---|
| `prompt_public` | 公开到社区画廊时是否同时公开提示词；默认 true；取消公开时重置为 true |

## 013_record_favorites

```sql
CREATE TABLE record_favorites (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    record_id   BIGINT NOT NULL REFERENCES records(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

| 列 | 说明 |
|---|---|
| `user_id` | 收藏公开图片的用户 |
| `record_id` | 被收藏的图片记录 |

## 014_site_title

```sql
ALTER TABLE upstream_configs
    ADD COLUMN site_title TEXT NOT NULL DEFAULT 'shadraw'
        CHECK (char_length(btrim(site_title)) BETWEEN 1 AND 64);
```

| 列 | 说明 |
|---|---|
| `site_title` | 管理员配置的网站标题；用于前端浏览器标题、顶部品牌和登录页品牌文案 |

## 迁移规则

- 工具：`golang-migrate/migrate v4`，文件名 `NNN_short_name.up.sql` / `.down.sql`。
- 每条迁移**可独立回滚**；本仓库 `migrate-down 1` 会撤销最近一条。
- 一旦合并到 `main` 不再修改既有迁移，纠错只能加新迁移。
- 不在迁移里写 seed；`admin` 引导由应用层的 `app.EnsureAdmin` 完成。
