-- 003_files.sql
-- 文件上传与对象存储元数据。文件内容只进入对象存储，数据库只保存可审计、
-- 可授权、可引用的元数据和对象存储定位信息。

CREATE TABLE IF NOT EXISTS files (
    id          VARCHAR(64)  PRIMARY KEY,
    org_id      VARCHAR(64)  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id  VARCHAR(64)  NOT NULL REFERENCES projects(id)      ON DELETE CASCADE,
    user_id     VARCHAR(64)  NOT NULL REFERENCES users(id)         ON DELETE RESTRICT,
    filename    VARCHAR(255) NOT NULL,
    mime_type   VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes  BIGINT       NOT NULL DEFAULT 0,
    sha256      VARCHAR(64)  NOT NULL DEFAULT '',
    purpose     VARCHAR(32)  NOT NULL
        CHECK (purpose IN ('knowledge_base','conversation','user_data')),
    status      VARCHAR(32)  NOT NULL
        CHECK (status IN ('uploading','uploaded','failed','deleted')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_files_project_time
    ON files (org_id, project_id, created_at DESC)
    WHERE status <> 'deleted';
CREATE INDEX IF NOT EXISTS idx_files_sha256
    ON files (org_id, project_id, sha256)
    WHERE status <> 'deleted';

CREATE TABLE IF NOT EXISTS file_objects (
    id               VARCHAR(64) PRIMARY KEY,
    file_id          VARCHAR(64) NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    storage_provider VARCHAR(32) NOT NULL,
    bucket           VARCHAR(255) NOT NULL,
    object_key       TEXT NOT NULL,
    etag             VARCHAR(255),
    version_id       VARCHAR(255),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_file_objects_file
    ON file_objects (file_id);

INSERT INTO schema_migrations (version)
VALUES ('003_files')
ON CONFLICT (version) DO NOTHING;
