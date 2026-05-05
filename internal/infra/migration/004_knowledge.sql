-- 004_knowledge.sql
-- 知识库、文档和 chunk 索引。文件内容仍在对象存储，数据库保存可检索文本、
-- embedding 和引用元数据。

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_bases (
    id               VARCHAR(64) PRIMARY KEY,
    org_id           VARCHAR(64) NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id       VARCHAR(64) NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             VARCHAR(128) NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    visibility       VARCHAR(32) NOT NULL DEFAULT 'project',
    access_policy    JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedding_model  VARCHAR(128) NOT NULL,
    embedding_dim    INTEGER NOT NULL DEFAULT 1536,
    chunk_config     JSONB NOT NULL DEFAULT '{}'::jsonb,
    retrieval_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    status           VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','deleted')),
    created_by       VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_project
    ON knowledge_bases (org_id, project_id, created_at DESC)
    WHERE status <> 'deleted';

CREATE TABLE IF NOT EXISTS documents (
    id                VARCHAR(64) PRIMARY KEY,
    org_id            VARCHAR(64) NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id        VARCHAR(64) NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(64) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    file_id           VARCHAR(64) NOT NULL REFERENCES files(id) ON DELETE RESTRICT,
    filename          VARCHAR(255) NOT NULL,
    mime_type         VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    status            VARCHAR(32) NOT NULL
        CHECK (status IN ('pending','processing','ready','failed','deleted')),
    parse_error       TEXT NOT NULL DEFAULT '',
    attempts          INTEGER NOT NULL DEFAULT 0,
    processing_updated_at TIMESTAMPTZ,
    chunk_count       INTEGER NOT NULL DEFAULT 0,
    char_count        INTEGER NOT NULL DEFAULT 0,
    token_count       INTEGER NOT NULL DEFAULT 0,
    created_by        VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_documents_kb_time
    ON documents (org_id, knowledge_base_id, created_at DESC)
    WHERE status <> 'deleted';
CREATE INDEX IF NOT EXISTS idx_documents_pending
    ON documents (status, processing_updated_at, updated_at)
    WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS document_chunks (
    id                VARCHAR(64) PRIMARY KEY,
    org_id            VARCHAR(64) NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id        VARCHAR(64) NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(64) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id       VARCHAR(64) NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    file_id           VARCHAR(64) NOT NULL REFERENCES files(id) ON DELETE RESTRICT,
    chunk_index       INTEGER NOT NULL,
    content           TEXT NOT NULL,
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- MVP 固定 text-embedding-3-small/mock 的 1536 维；换 3072 维等模型时需要新列或重建表和索引。
    embedding         vector(1536) NOT NULL,
    keyword           tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
    status            VARCHAR(32) NOT NULL DEFAULT 'ready'
        CHECK (status IN ('ready','deleted')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_document_chunks_doc_index
    ON document_chunks (document_id, chunk_index);
CREATE INDEX IF NOT EXISTS idx_document_chunks_scope
    ON document_chunks (org_id, project_id, knowledge_base_id, document_id, status);
CREATE INDEX IF NOT EXISTS idx_document_chunks_keyword
    ON document_chunks USING GIN (keyword);
CREATE INDEX IF NOT EXISTS idx_document_chunks_embedding_hnsw
    ON document_chunks USING hnsw (embedding vector_cosine_ops);

INSERT INTO schema_migrations (version)
VALUES ('004_knowledge')
ON CONFLICT (version) DO NOTHING;
