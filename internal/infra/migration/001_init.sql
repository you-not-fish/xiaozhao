-- 001_init.sql
-- MVP baseline: users / organizations / org_members / projects.
-- Other tables (conversations, messages, tool_definitions, ...) will be added
-- in later migrations as their owning modules land.

CREATE TABLE IF NOT EXISTS users (
    id                VARCHAR(64)  PRIMARY KEY,
    email             VARCHAR(255) NOT NULL,
    password_hash     VARCHAR(255) NOT NULL,
    name              VARCHAR(255) NOT NULL,
    status            VARCHAR(32)  NOT NULL DEFAULT 'active',
    -- reserved for future SSO / OIDC; keeps MVP from rewriting the users table later.
    external_id       VARCHAR(255),
    identity_provider VARCHAR(64),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_active
    ON users (lower(email))
    WHERE status <> 'deleted';

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_external_identity
    ON users (identity_provider, external_id)
    WHERE external_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS organizations (
    id         VARCHAR(64)  PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    plan       VARCHAR(64)  NOT NULL DEFAULT 'free',
    status     VARCHAR(32)  NOT NULL DEFAULT 'active',
    created_by VARCHAR(64)  NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_organizations_created_by ON organizations (created_by);

CREATE TABLE IF NOT EXISTS org_members (
    org_id    VARCHAR(64)  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id   VARCHAR(64)  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      VARCHAR(32)  NOT NULL CHECK (role IN ('owner','admin','member','viewer')),
    status    VARCHAR(32)  NOT NULL DEFAULT 'active',
    joined_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members (user_id);

CREATE TABLE IF NOT EXISTS projects (
    id         VARCHAR(64)  PRIMARY KEY,
    org_id     VARCHAR(64)  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name       VARCHAR(255) NOT NULL,
    settings   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    -- visibility is reserved for future per-KB/project ACLs; MVP always uses 'org'.
    visibility VARCHAR(32)  NOT NULL DEFAULT 'org',
    created_by VARCHAR(64)  NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects (org_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_org_name ON projects (org_id, lower(name));

-- Migration bookkeeping table so this file can be replayed idempotently.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    VARCHAR(64) PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_migrations (version)
VALUES ('001_init')
ON CONFLICT (version) DO NOTHING;
