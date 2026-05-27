# shadraw API - v1

> Auth、records、projects、admin runtime 的 v1 contract 摘要。

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
| `upstream_error` | 上游接口异常（502） |

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

## GET /config

返回前端启动所需的公开配置。

- 鉴权：无
- 200 响应：

```json
{
  "data": {
    "enabledModels": ["gpt-image-2"],
    "siteTitle": "shadraw"
  },
  "error": null
}
```

---

## POST /records

创建一条生图任务。图片参数统一使用 OpenAI 官方字段 `imageParams`。

- 鉴权：Bearer
- 请求体：

```json
{
  "prompt": "a cinematic product photo of a red chair",
  "model": "gpt-image-2",
  "imageParams": {
    "size": "1536x1024",
    "quality": "high",
    "background": "auto",
    "moderation": "auto",
    "output_format": "png",
    "output_compression": 90,
    "stream": false,
    "partial_images": 0,
    "input_fidelity": "high",
    "response_format": "b64_json",
    "style": "natural",
    "user": "user-12"
  },
  "referenceImages": ["data:image/png;base64,..."],
  "projectId": "7"
}
```

- 201 响应：

```json
{
  "data": {
    "record": {
      "id": "42",
      "uuid": "3a5f...",
      "prompt": "a cinematic product photo of a red chair",
      "model": "gpt-image-2",
      "imageParams": {
        "size": "1536x1024",
        "quality": "high",
        "background": "auto",
        "moderation": "auto",
        "output_format": "png"
      },
      "status": "waiting",
      "favorite": false,
      "isPublic": false,
      "promptPublic": true,
      "hasImage": false,
      "error": "提示词被安全系统拒绝，请调整提示词后重试",
      "upstreamError": "upstream bad_request (400): invalid_request_error: ...",
      "referenceCount": 1,
      "createdAt": "2026-05-26T11:08:00Z"
    }
  },
  "error": null
}
```

说明：当前产品只支持每条任务生成一张图片。后端会把 `imageParams.n` 固定归一为 `1`，前端不提供图片数量设置。

失败记录会返回面向用户的 `error`。如果上游返回了可解析错误，当前用户自己的记录还会返回 `upstreamError` 作为调试详情；社区公开列表不会暴露该字段。

## GET /records

- 鉴权：Bearer
- Query：`status`, `projectId`, `favorite`, `scope`, `q`, `page`, `pageSize`
- `scope` 默认是当前用户记录；`scope=public` 返回社区公开画廊，限定为已完成且有图片的公开记录。若记录 `promptPublic=false`，社区列表中的 `prompt` 返回空字符串。社区列表中的 `favorite` 表示当前用户是否收藏该公开图片。
- `q` 按提示词模糊搜索；社区公开列表只搜索 `promptPublic=true` 的记录，避免用私密提示词命中结果。
- `projectId=none` 表示未分类记录。
- 200 响应：`{ "data": { "records": [RecordDTO] }, "error": null, "meta": ... }`

## GET /records/:id

- 鉴权：Bearer
- 200 响应：`{ "data": { "record": RecordDTO }, "error": null }`

## PATCH /records/:id

- 鉴权：Bearer
- 请求体：`{ "favorite": true, "isPublic": true, "promptPublic": true, "projectId": "7" }`
- `favorite` 对自己的记录更新记录字段；对他人的公开图片写入当前用户的收藏关系，不影响作者自己的收藏状态。
- `projectId: ""` 或 `null` 表示移出项目。
- `isPublic: true` 会把已完成图片发布到社区画廊；新生成图片默认 `isPublic=false`。
- `promptPublic` 仅在公开图片时生效，表示是否同时向社区公开提示词；公开时省略该字段按 `false` 处理，取消公开会重置为 `true`。

## POST /records/:id/retry

- 鉴权：Bearer
- 仅失败记录可重试。

## GET /images/:id

- 鉴权：Bearer
- 返回该 record 的图片二进制。
- 当前用户可读取自己的图片；其他用户只能读取已公开的图片。

---

## GET /admin/site-settings

读取站点设置。

- 鉴权：Bearer + admin
- 200 响应：

```json
{
  "data": {
    "config": {
      "siteTitle": "shadraw"
    }
  },
  "error": null
}
```

## PATCH /admin/site-settings

更新站点标题。

- 鉴权：Bearer + admin
- 请求体：

```json
{ "siteTitle": "我的生图站" }
```

- 200 响应：同 `GET /admin/site-settings`
- 422：`siteTitle` 为空或超过 64 个字符。
