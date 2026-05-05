-- 002_agent.sql
-- Agent 事件化协议相关表：会话、消息、响应事件、工具定义/调用、模型调用、
-- trace/审计/用量。每张表都带 org_id 和 project_id 以贯穿多租户链路。
-- 设计要点：
-- 1. response_events 使用 (response_id, seq) 唯一，保证前端可按序回放；
-- 2. tool_calls 与 tool_results 分两表，便于未来大结果走对象存储；
-- 3. trace_spans 和 audit_logs 分离，trace 用于调试，audit 用于合规追责；
-- 4. model_invocations 与 usage_records 分离，前者是技术调用审计，后者是成本聚合。

-- ========== 工具定义 ==========
-- 存储 builtin 工具与未来接入的 http/mcp/sandbox 工具，统一走 Tool Router。
CREATE TABLE IF NOT EXISTS tool_definitions (
    id                   VARCHAR(64)  PRIMARY KEY,
    org_id               VARCHAR(64),                         -- NULL 表示全局内置工具
    project_id           VARCHAR(64),                         -- NULL 表示组织级
    name                 VARCHAR(128) NOT NULL,               -- 工具唯一名，模型通过此调用
    description          TEXT         NOT NULL,
    provider_type        VARCHAR(32)  NOT NULL DEFAULT 'builtin'
        CHECK (provider_type IN ('builtin','http','mcp','sandbox')),
    category             VARCHAR(64)  NOT NULL DEFAULT 'misc',
    risk_level           VARCHAR(8)   NOT NULL DEFAULT 'L0'
        CHECK (risk_level IN ('L0','L1','L2','L3')),
    requires_confirmation BOOLEAN     NOT NULL DEFAULT FALSE,  -- 高风险工具是否需要审批
    approval_policy      VARCHAR(64),                         -- 预留审批策略 key
    input_schema         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    output_schema        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    timeout_ms           INTEGER      NOT NULL DEFAULT 15000,
    enabled              BOOLEAN      NOT NULL DEFAULT TRUE,
    auth_config          JSONB        NOT NULL DEFAULT '{}'::jsonb,  -- 凭据引用，不存明文
    created_by           VARCHAR(64),
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tool_definitions_scope
    ON tool_definitions (org_id, project_id, enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_definitions_name_scope
    ON tool_definitions (COALESCE(org_id,''), COALESCE(project_id,''), name);

-- ========== 会话 ==========
CREATE TABLE IF NOT EXISTS conversations (
    id         VARCHAR(64)  PRIMARY KEY,
    org_id     VARCHAR(64)  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id VARCHAR(64)  NOT NULL REFERENCES projects(id)      ON DELETE CASCADE,
    user_id    VARCHAR(64)  NOT NULL REFERENCES users(id)         ON DELETE RESTRICT,
    title      VARCHAR(255) NOT NULL DEFAULT '',
    status     VARCHAR(32)  NOT NULL DEFAULT 'active',
    metadata   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_conversations_project_user
    ON conversations (project_id, user_id, updated_at DESC);

-- ========== 消息与消息项 ==========
-- messages 存 role=user/assistant/system/tool 的父记录，
-- message_items 存其下的文本/工具调用/引用等子项，便于 UI 按 item 渲染。
CREATE TABLE IF NOT EXISTS messages (
    id              VARCHAR(64) PRIMARY KEY,
    conversation_id VARCHAR(64) NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    response_id     VARCHAR(64),                              -- assistant 消息对应的 response
    role            VARCHAR(16) NOT NULL
        CHECK (role IN ('user','assistant','system','tool')),
    status          VARCHAR(32) NOT NULL DEFAULT 'completed',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_created
    ON messages (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_response ON messages (response_id);

CREATE TABLE IF NOT EXISTS message_items (
    id         VARCHAR(64) PRIMARY KEY,
    message_id VARCHAR(64) NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    seq        INTEGER     NOT NULL DEFAULT 0,
    type       VARCHAR(32) NOT NULL
        CHECK (type IN ('text','tool_call','tool_result','citation','error')),
    content    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    metadata   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_message_items_msg ON message_items (message_id, seq);

-- ========== 响应事件（event sourcing 核心表） ==========
-- 每个 /v1/responses 请求产生一条 response 对应一条 assistant message，
-- 其事件流完整持久化到此表，供 SSE 客户端断点重连与后台回放。
CREATE TABLE IF NOT EXISTS response_events (
    id              VARCHAR(64) PRIMARY KEY,
    response_id     VARCHAR(64) NOT NULL,
    conversation_id VARCHAR(64) NOT NULL,
    org_id          VARCHAR(64) NOT NULL,
    project_id      VARCHAR(64) NOT NULL,
    seq             INTEGER     NOT NULL,                     -- 从 1 开始单调递增
    type            VARCHAR(64) NOT NULL,                     -- 见 EventType 枚举
    data            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 同一 response 内 seq 唯一；Postgres 会按此索引高效拉取有序事件。
CREATE UNIQUE INDEX IF NOT EXISTS idx_response_events_seq
    ON response_events (response_id, seq);
CREATE INDEX IF NOT EXISTS idx_response_events_conv
    ON response_events (conversation_id, created_at);

-- ========== 模型调用审计 ==========
CREATE TABLE IF NOT EXISTS model_invocations (
    id             VARCHAR(64) PRIMARY KEY,
    org_id         VARCHAR(64) NOT NULL,
    project_id     VARCHAR(64) NOT NULL,
    response_id    VARCHAR(64) NOT NULL,
    model_name     VARCHAR(128) NOT NULL,
    provider       VARCHAR(64)  NOT NULL,
    input_tokens   INTEGER      NOT NULL DEFAULT 0,
    output_tokens  INTEGER      NOT NULL DEFAULT 0,
    latency_ms     INTEGER      NOT NULL DEFAULT 0,
    status         VARCHAR(32)  NOT NULL,
    error_code     VARCHAR(64),
    estimated_cost NUMERIC(18,8) NOT NULL DEFAULT 0,          -- 估算美元成本
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_model_invocations_response ON model_invocations (response_id);
CREATE INDEX IF NOT EXISTS idx_model_invocations_org_time ON model_invocations (org_id, created_at DESC);

-- ========== 工具调用审计 ==========
CREATE TABLE IF NOT EXISTS tool_calls (
    id            VARCHAR(64) PRIMARY KEY,
    org_id        VARCHAR(64) NOT NULL,
    project_id    VARCHAR(64) NOT NULL,
    response_id   VARCHAR(64) NOT NULL,
    tool_id       VARCHAR(64),                                -- 可空：内置工具没有 tool_definitions 行
    tool_name     VARCHAR(128) NOT NULL,
    args_hash     VARCHAR(64)  NOT NULL DEFAULT '',           -- sha256，便于去重/检索
    args_redacted JSONB        NOT NULL DEFAULT '{}'::jsonb,  -- 脱敏后的参数快照
    status        VARCHAR(32)  NOT NULL,                      -- pending/running/succeeded/failed
    latency_ms    INTEGER      NOT NULL DEFAULT 0,
    error_code    VARCHAR(64),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tool_calls_response ON tool_calls (response_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_org_time ON tool_calls (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS tool_results (
    id              VARCHAR(64) PRIMARY KEY,
    tool_call_id    VARCHAR(64) NOT NULL REFERENCES tool_calls(id) ON DELETE CASCADE,
    result_ref      VARCHAR(255),                             -- 对象存储 ref，大结果使用
    result_summary  TEXT         NOT NULL DEFAULT '',
    result_redacted JSONB        NOT NULL DEFAULT '{}'::jsonb,
    truncated       BOOLEAN      NOT NULL DEFAULT FALSE,      -- 是否触发 64KB 截断
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tool_results_call ON tool_results (tool_call_id);

-- ========== trace / audit / usage ==========
-- trace_spans：用于性能与调试，关联整个 Agent run 内的所有 span。
CREATE TABLE IF NOT EXISTS trace_spans (
    id                  VARCHAR(64) PRIMARY KEY,
    trace_id            VARCHAR(64) NOT NULL,
    parent_span_id      VARCHAR(64),
    org_id              VARCHAR(64) NOT NULL,
    project_id          VARCHAR(64) NOT NULL,
    response_id         VARCHAR(64),
    span_type           VARCHAR(32) NOT NULL,                 -- agent_run/model/tool/guardrail/rag
    name                VARCHAR(128) NOT NULL,
    status              VARCHAR(32) NOT NULL DEFAULT 'ok',
    started_at          TIMESTAMPTZ NOT NULL,
    ended_at            TIMESTAMPTZ,
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    attributes_redacted JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_trace_spans_trace ON trace_spans (trace_id, started_at);
CREATE INDEX IF NOT EXISTS idx_trace_spans_response ON trace_spans (response_id);

-- audit_logs：合规追责用，与 trace 分离。
CREATE TABLE IF NOT EXISTS audit_logs (
    id            VARCHAR(64) PRIMARY KEY,
    org_id        VARCHAR(64),
    project_id    VARCHAR(64),
    actor_user_id VARCHAR(64),
    action        VARCHAR(128) NOT NULL,
    resource_type VARCHAR(64)  NOT NULL,
    resource_id   VARCHAR(64),
    ip            VARCHAR(64),
    user_agent    VARCHAR(255),
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_time ON audit_logs (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs (actor_user_id, created_at DESC);

-- usage_records：细粒度用量流水，未来由 UsageWorker 聚合到账单。
CREATE TABLE IF NOT EXISTS usage_records (
    id             VARCHAR(64) PRIMARY KEY,
    org_id         VARCHAR(64) NOT NULL,
    project_id     VARCHAR(64) NOT NULL,
    user_id        VARCHAR(64),
    source_type    VARCHAR(32) NOT NULL,                      -- model/tool/search/embedding
    source_id      VARCHAR(64),                               -- model_invocation/tool_call id
    quantity       NUMERIC(18,4) NOT NULL DEFAULT 0,
    unit           VARCHAR(32)   NOT NULL DEFAULT 'token',
    estimated_cost NUMERIC(18,8) NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_usage_records_org_time ON usage_records (org_id, created_at DESC);

INSERT INTO schema_migrations (version)
VALUES ('002_agent')
ON CONFLICT (version) DO NOTHING;
