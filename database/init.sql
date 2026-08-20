-- =============================================================================
-- 数据库初始化脚本（仅在 PostgreSQL 容器首次创建时执行）
-- 约定：
--   1. 每个业务表都有 uuid（gen_random_uuid 自动生成）用于外部引用
--   2. 带默认值的字段一律 NOT NULL，避免脏数据
--   3. 不使用 ON DELETE CASCADE（担心删除主数据时级联误删，一律 NO ACTION）
-- =============================================================================

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
    username      VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    email         VARCHAR(100),
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 命令记录表（无 users 外键：MCP 写入的是 QQ 用户 ID，不在 users 表）
CREATE TABLE IF NOT EXISTS commands (
    id            SERIAL PRIMARY KEY,
    uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id       INTEGER,
    source        VARCHAR(20) NOT NULL DEFAULT 'qq',
    command_text  TEXT NOT NULL,
    command_type  VARCHAR(30) NOT NULL DEFAULT 'shell',
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    result        TEXT,
    error_message TEXT,
    exit_code     INTEGER,
    environment   JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMP
);

-- 邮件对象表（遗留，暂未使用；外键为 NO ACTION，无级联）
CREATE TABLE IF NOT EXISTS email_objects (
    id            SERIAL PRIMARY KEY,
    user_id       INTEGER REFERENCES users(id),
    name          VARCHAR(100),
    email_address VARCHAR(255) NOT NULL,
    company       VARCHAR(200),
    position      VARCHAR(200),
    tags          JSONB DEFAULT '[]',
    notes         TEXT,
    metadata      JSONB DEFAULT '{}',
    is_active     BOOLEAN DEFAULT TRUE,
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

-- 邮件发送记录表（遗留，暂未使用；外键为 NO ACTION，无级联）
CREATE TABLE IF NOT EXISTS email_records (
    id            SERIAL PRIMARY KEY,
    user_id       INTEGER REFERENCES users(id),
    object_id     INTEGER REFERENCES email_objects(id),
    to_address    VARCHAR(255) NOT NULL,
    cc_address    TEXT,
    bcc_address   TEXT,
    subject       VARCHAR(500) NOT NULL,
    body          TEXT,
    body_html     TEXT,
    attachments   JSONB DEFAULT '[]',
    status        VARCHAR(20) DEFAULT 'pending',
    error_message TEXT,
    sent_at       TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW()
);

-- JWT Token 黑名单（登出 / refresh）
CREATE TABLE IF NOT EXISTS token_blacklist (
    id            SERIAL PRIMARY KEY,
    uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
    token_jti     VARCHAR(255) UNIQUE NOT NULL,
    expires_at    TIMESTAMP NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 操作存档表（Go MCP 服务器写入，后端只读展示）
CREATE TABLE IF NOT EXISTS operations (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
    session_id    TEXT,
    tool_name     TEXT NOT NULL,
    file_path     TEXT,
    params        JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at DESC);

-- 文件元数据表（描述/标签/属性/软删除）
CREATE TABLE IF NOT EXISTS file_metadata (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID NOT NULL DEFAULT gen_random_uuid(),
    file_path        TEXT UNIQUE NOT NULL,
    scope            TEXT NOT NULL DEFAULT 'global',
    owner_id         TEXT,
    title            TEXT,
    description      TEXT,
    tags             TEXT[] DEFAULT '{}',
    file_type        TEXT,
    mime_type        TEXT,
    extension        TEXT,
    size_bytes       BIGINT,
    checksum         TEXT,
    session_id       TEXT,
    user_id          TEXT,
    attributes       JSONB NOT NULL DEFAULT '{}',
    copied_from      TEXT,
    download_count   BIGINT NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    is_deleted       BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_meta_tags  ON file_metadata USING GIN (tags);
CREATE INDEX IF NOT EXISTS idx_meta_attrs ON file_metadata USING GIN (attributes);
CREATE INDEX IF NOT EXISTS idx_meta_type  ON file_metadata (file_type);
CREATE INDEX IF NOT EXISTS idx_meta_by    ON file_metadata (user_id);
CREATE INDEX IF NOT EXISTS idx_meta_del   ON file_metadata (is_deleted);
