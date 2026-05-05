# xiaozhao — AI Agent 团队知识助理 MVP 后端

这是 `docs/MVP需求与技术实现方案.md` 描述的 Go 模块化单体后端。
当前仓库已完成 **P0 Agent 核心闭环**、**P1 文件上传/对象存储基础能力** 与 **P1 知识库检索闭环**：账号、组织、项目、RBAC、Responses SSE、模型/工具循环、审计/用量记录、文件元数据、S3-compatible 对象存储、文档解析、chunk、embedding、pgvector 检索和 `knowledge_search` 工具。

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
    agent/                 Responses SSE、会话、Agent Orchestrator
    file/                  文件上传、下载、软删除、对象存储编排
    knowledge/             知识库、文档入库、chunk、检索编排
    org/                   组织创建、成员增删改查、RBAC 入口
    project/               项目 CRUD
    rbac/                  基于 Role 等级的授权中心
  domain/                  实体与仓储接口
  gateway/
    httpctx/               请求上下文键（request_id / user_id / org_id）
    middleware/            request_id / logging / recovery / auth / tenant / ratelimit
  infra/
    cache/                 Redis 客户端
    database/              PostgreSQL 连接池 + GORM 日志桥接 zap
    embedding/             Embedding Provider（mock / OpenAI-compatible）
    migration/             SQL 迁移 + embed（schema_migrations 记录版本）
    model/                 模型 Provider / Router 适配
    parser/                TXT / Markdown / CSV / DOCX / PDF 基础文本解析
    repository/            GORM 仓储实现（domain 接口的具体实现）
    storage/               S3-compatible / MinIO 对象存储抽象
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
- PostgreSQL 14+ with pgvector（开发环境 docker compose 使用 `pgvector/pgvector:pg16`）
- Redis 7+（可选，不可用时限流降级为放行）
- MinIO 或 S3-compatible 对象存储（开发环境 docker compose 已包含 MinIO）

最简启动：

```bash
make docker-up     # 启动 postgres + redis + minio
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
| POST /v1/responses                          | 创建 Agent 响应（SSE） | Bearer    | X-Organization-Id 或 JWT 内 |
| GET  /v1/responses/{responseID}/events      | 回放响应事件            | Bearer    | 同上  |
| POST /v1/files                              | multipart 上传文件      | Bearer    | 同上  |
| GET  /v1/files?project_id=...               | 列出项目文件            | Bearer    | 同上  |
| GET  /v1/files/{fileID}                     | 获取文件元数据          | Bearer    | 同上  |
| GET  /v1/files/{fileID}/content             | 后端鉴权代理下载文件    | Bearer    | 同上  |
| DELETE /v1/files/{fileID}                   | 软删除文件并清理对象    | Bearer    | 同上  |
| POST /v1/knowledge_bases                    | 创建知识库（admin+）    | Bearer    | 同上  |
| GET  /v1/knowledge_bases?project_id=...     | 列出项目知识库          | Bearer    | 同上  |
| GET  /v1/knowledge_bases/{kbID}             | 获取知识库              | Bearer    | 同上  |
| POST /v1/knowledge_bases/{kbID}/documents   | 挂载文件并异步入库      | Bearer    | 同上  |
| GET  /v1/knowledge_bases/{kbID}/documents   | 查看文档入库状态        | Bearer    | 同上  |
| POST /v1/knowledge_bases/{kbID}/search      | 检索知识库 chunk        | Bearer    | 同上  |

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
- `app/agent` P0 状态机：无工具、工具调用、工具失败、工具结果截断、模型失败
- `api/v1` responses handler：鉴权、租户上下文、SSE header、事件回放租户校验
- `app/file` 文件上传：大小限制、空文件拒绝、MIME sniff、对象存储失败、软删除、跨租户不可见
- `api/v1` files handler：鉴权、租户上下文、multipart 校验、文件下载、跨租户不可见
- `infra/parser` 文档解析：TXT/Markdown、CSV、DOCX、基础 PDF 文本抽取、空文档拒绝
- `app/knowledge`：知识库创建、文件挂载、文档状态流转、chunk 入库、租户隔离、检索
- `infra/embedding`：mock embedding 维度稳定和确定性

## 下一步建议

已完成：

1. `POST /v1/responses` SSE demo + `response_events` 表（事件先持久化再推）
2. Model Gateway（mock / OpenAI-compatible）与内置 `current_time`、`calculator` 工具
3. 基础 trace/span 记录（`trace_spans` 表 + `internal/observability`）
4. 模型调用、工具调用、审计日志、用量流水仓储
5. `POST /v1/files` 文件上传、`files/file_objects` 元数据、MinIO/S3-compatible 存取、后端代理下载、软删除
6. `POST /v1/knowledge_bases`、文档挂载、Go 文本解析、chunk、mock/OpenAI-compatible embedding、pgvector 检索
7. `knowledge_search` 内置工具，按 response 请求中的 `knowledge_base_ids` 收窄检索范围

后续按 RAG 质量和运营能力推进：

1. 更高保真 PDF/DOCX 解析、表格结构化和 OCR，可通过 Python parser worker 扩展
2. Rerank、检索评测集、召回率/引用准确率回归
3. `web_search` 工具
4. 管理后台 API：用量、审计、tool calls、model invocations、trace 查询
