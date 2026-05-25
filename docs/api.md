# shadraw API — v1

> 第 1 轮 MVP 范围：Auth 6 个接口。后续轮次会扩展 records / projects / admin。

所有接口遵循 [接口规范](https://github.com/liusx/shadraw-ui/blob/main/.trellis/tasks/05-25-shadraw-backend-bootstrap/design.md#10-接口设计规范本轮强制落实)（响应外壳、错误码、状态码、ID 字符串化、时间 UTC）。

## 通用约定

- Base URL：`http://localhost:8080/api/v1`（生产替换为部署 origin）
- 响应外壳：`{ "data": <T>, "error": null | { code, message, fields? }, "meta"?: ... }`
- 鉴权：受保护接口需 `Authorization: Bearer <accessToken>`
- ID：JSON 中**永远是字符串**

## 错误码

| code | 含义 |
|---|---|
| `validation_failed` | 请求参数不合法（422） |
| `unauthorized` | 未登录 / 凭证错 / refresh 无效（401） |
| `forbidden` | 已登录但无权访问 / 账号禁用（403） |
| `account_disabled` | 账号被管理员禁用（403，登录与 RequireAuth 用） |
| `not_found` | 资源不存在（404） |
| `conflict` | 资源冲突，如邮箱已注册（409） |
| `rate_limited` | 命中限流（429，含 `Retry-After` 头） |
| `internal_error` | 服务端异常（500） |

---

## POST /auth/register

注册新账号。

- 鉴权：无
- 限流：5/min/IP
- 请求体：

```json
{
  "email": "alice@example.com",
  "password": "hunter2pass",
  "displayName": "alice"
}
```

- 201 响应：

```json
{
  "data": {
    "user": {
      "id": "12",
      "email": "alice@example.com",
      "displayName": "alice",
      "role": "user",
      "mustChangePassword": false,
      "createdAt": "2026-05-25T11:08:00Z"
    },
    "tokens": {
      "accessToken": "eyJhbGc...",
      "refreshToken": "tR2HfL...",
      "expiresIn": 900
    }
  },
  "error": null
}
```

- 409（邮箱已注册）：

```json
{ "data": null, "error": { "code": "conflict", "message": "邮箱已被注册" } }
```

- 422（校验失败）：

```json
{
  "data": null,
  "error": {
    "code": "validation_failed",
    "message": "参数校验失败",
    "fields": { "email": "邮箱格式不合法", "password": "至少 8 个字符" }
  }
}
```

---

## POST /auth/login

登录。

- 鉴权：无
- 限流：5/min/IP
- 请求体：

```json
{ "email": "alice@example.com", "password": "hunter2pass" }
```

- 200 响应：与 register 同结构。
- 401（邮箱或密码错）：`{ "error": { "code": "unauthorized", "message": "邮箱或密码错误" } }`
- 403（账号禁用）：`{ "error": { "code": "account_disabled", "message": "账号已禁用" } }`

---

## POST /auth/refresh

用 refresh token 换取新的 access + refresh 对（rotation）。**老的 refresh 立即失效**。

- 鉴权：无
- 限流：60/min/IP
- 请求体：`{ "refreshToken": "tR2HfL..." }`
- 200 响应：

```json
{
  "data": {
    "tokens": {
      "accessToken": "eyJhbGc...",
      "refreshToken": "newRefresh...",
      "expiresIn": 900
    }
  },
  "error": null
}
```

- 401：`{ "error": { "code": "unauthorized", "message": "refresh token 无效" } }`（包含 invalid / expired / revoked 三种情况）

---

## POST /auth/logout

撤销指定的 refresh token。

- 鉴权：Bearer
- 限流：60/min/user
- 请求体：`{ "refreshToken": "tR2HfL..." }`
- 200 响应：`{ "data": { "ok": true }, "error": null }`
- 未知 token 视为成功（幂等）。

---

## GET /auth/me

返回当前登录用户。

- 鉴权：Bearer
- 200 响应：

```json
{ "data": { "user": { "id": "12", "email": "alice@example.com", ... } }, "error": null }
```

- 401：缺 token / token 过期 / 用户已删除。
- 403：账号已禁用。

---

## POST /auth/password

修改密码。验证旧密码后写入新密；**所有 refresh token 被立即撤销**。

- 鉴权：Bearer
- 限流：10/min/user
- 请求体：

```json
{ "oldPassword": "hunter2pass", "newPassword": "newSecret9" }
```

- 200 响应：`{ "data": { "ok": true }, "error": null }`
- 401（旧密码错）：`{ "error": { "code": "unauthorized", "message": "旧密码错误" } }`
- 422（新密码太短）：`fields.newPassword = "至少 8 个字符"`

---

## 健康检查

`GET /healthz` → `200 { "data": { "status": "ok" }, "error": null }`，不带 v1 前缀。
