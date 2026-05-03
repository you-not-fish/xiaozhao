# xiaozhao — AI Agent 团队知识助理 MVP 后端

这是 `docs/MVP需求与技术实现方案.md` 描述的 Go 模块化单体后端。
当前仓库完成了 **P0 前置：工程骨架 + 账号/组织/项目/RBAC** 的闭环，为后续会话、知识库、工具编排、模型网关、审计追溯打好租户隔离基础。

## 目录结构

```
cmd/server/                服务入口（装配所有组件并启动 HTTP）
configs/                   配置文件样例（YAML）
scripts/                   docker-compose 等运维脚本

internal/
  api/
    dto/                   REST JSON 请求/响应 DTO 与 domain 映射
    response/              统一 JSON Envelope（data / error / request_id）
    router/                中间件与路由装配
    v1/                    HTTP handler（薄层：解码 → 调用服务 → 映射 DTO）
  app/
    auth/                  注册、登录、/me、切换组织
    org/                   组织创建、成员增删改查、RBAC 入口
    project/               项目 CRUD
    rbac/                  基于 Role 等级的授权中心
  domain/                  实体与仓储接口（User / Organization / OrgMember / Project）
  gateway/
    httpctx/               请求上下文键（request_id / user_id / org_id）
    middleware/            request_id / logging / recovery / auth / tenant / ratelimit
  infra/
    cache/                 Redis 客户端
    database/              PostgreSQL 连接池 + GORM 日志桥接 zap
    migration/             SQL 迁移 + embed（schema_migrations 记录版本）
    repository/            GORM 仓储实现（domain 接口的具体实现）
  pkg/
    config/                viper 配置加载
    errcode/               结构化错误码 + HTTP 映射
    id/                    `{prefix}_{ulid}` 资源 ID（user_, org_, proj_, conv_, ...）
    jwt/                   HS256 JWT 签发/校验
    logger/                zap Logger + context 注入
    password/              bcrypt
  observability/           预留：trace/metrics 在 P0 后续完成
  policy/                  预留：Guardrails 边界
  worker/                  预留：文档解析 / embedding / 网页抓取
```

## 运行前置

- Go 1.23+
- PostgreSQL 14+（MVP 期间不需要 pgvector，P1 引入）
- Redis 7+（可选，不可用时限流降级为放行）

最简启动：

```bash
make docker-up     # 启动 postgres + redis
make run           # 默认读取 configs/config.yaml
```

首次启动会自动执行 `internal/infra/migration/*.sql`（受 `postgres.auto_migrate` 控制）。

## API 一览

| 方法 + 路径 | 说明 | 鉴权 | 租户上下文 |
| --- | --- | --- | --- |
| GET  /healthz                               | 健康检查                | —         | —     |
| POST /v1/auth/register                      | 注册（直接签发 token）  | —         | —     |
| POST /v1/auth/login                         | 登录                    | —         | —     |
| GET  /v1/auth/me                            | 当前用户                | Bearer    | —     |
| POST /v1/auth/switch_org                    | 切换活跃组织重签 token  | Bearer    | —     |
| GET  /v1/orgs                               | 列出用户所属组织        | Bearer    | —     |
| POST /v1/orgs                               | 创建组织（自动成为 owner）| Bearer  | —     |
| GET  /v1/orgs/{orgID}                       | 查看组织               | Bearer    | —     |
| PATCH /v1/orgs/{orgID}                      | 重命名（admin+）        | Bearer    | —     |
| GET  /v1/orgs/{orgID}/members               | 成员列表                | Bearer    | —     |
| POST /v1/orgs/{orgID}/members               | 邀请 / 改角色（admin+） | Bearer    | —     |
| DELETE /v1/orgs/{orgID}/members/{userID}    | 移除成员（admin+）      | Bearer    | —     |
| POST /v1/projects                           | 创建项目（admin+）      | Bearer    | X-Organization-Id 或 JWT 内 |
| GET  /v1/projects                           | 列出项目                | Bearer    | 同上  |
| GET  /v1/projects/{projectID}               | 获取项目                | Bearer    | —     |
| PATCH /v1/projects/{projectID}              | 更新（admin+）          | Bearer    | —     |
| DELETE /v1/projects/{projectID}             | 删除（admin+）          | Bearer    | —     |

统一响应 Envelope：

```json
{ "data": { ... }, "error": null, "request_id": "req_01H..." }
```

错误：

```json
{
  "data": null,
  "error": { "code": "insufficient_role", "message": "role member is below required admin" },
  "request_id": "req_01H..."
}
```

## 关键设计取舍

- **ID 前缀化**：`user_`、`org_`、`proj_`、`conv_`、`resp_`、`evt_`、`tool_`、`call_`、`kb_`、`doc_` 等前缀全部在 `internal/pkg/id` 声明。后续会话、事件、工具调用直接复用，避免 ID 类型歧义。
- **领域/仓储分层**：`internal/domain` 只有纯 Go struct + 接口，GORM 的 `gorm:` tag 只出现在 `internal/infra/repository`。当 Event Store 或冷热分层要求换 pgx 时，只动 infra。
- **统一错误码**：`errcode.Error` 同时承载 `Code`、`Message`、`HTTP` 状态、`Metadata`。handler 层只负责 `response.WriteError`，不做类型转换。
- **Role 等级比较**：`Role.Rank()` / `Role.AtLeast()`，不允许字符串比较。owner > admin > member > viewer。`role="bogus"` 永远失败，不可能意外提权。
- **租户解析**：`X-Organization-Id` header 覆盖 JWT 内的 `org_id`，切换组织支持两种方式。`internal/gateway/middleware/tenant.go` 做 membership 检查后写入 `httpctx.OrgID`。
- **预留字段**：
  - `users.external_id / identity_provider` 为未来 SSO 预留
  - `projects.visibility` 为未来 per-KB/ABAC 预留
  - `schema_migrations` 独立，避免后续换 go-migrate 时数据割裂

## 测试

```bash
go test ./... -race
```

当前覆盖：
- `domain.Role.AtLeast` / `Valid`
- `pkg/id` 生成、解析、前缀校验
- `pkg/errcode` HTTP 映射与 `errors.Is` 穿透
- `pkg/jwt` 签发/校验 round-trip、错密钥、过期
- `pkg/password` bcrypt round-trip、空输入拒绝

## 下一步建议

按 MVP 文档 §9.1 P0 的剩余交付顺序推进：

1. `POST /v1/responses` SSE demo + `response_events` 表（事件先持久化再推）
2. Model Gateway（OpenAI-compatible，内置 `current_time`、`calculator` 工具）
3. 基础 trace/span 记录（`trace_spans` 表 + `internal/observability`）
4. 审计日志（`audit_logs`） + 用量（`usage_records`） 的仓储
