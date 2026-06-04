-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    username      VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    email         VARCHAR(100),
    is_active     BOOLEAN DEFAULT TRUE,
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

-- 命令记录表
CREATE TABLE IF NOT EXISTS commands (
    id            SERIAL PRIMARY KEY,
    user_id       INTEGER REFERENCES users(id),
    source        VARCHAR(20) DEFAULT 'qq',
    command_text  TEXT NOT NULL,
    command_type  VARCHAR(30) DEFAULT 'shell',
    status        VARCHAR(20) DEFAULT 'pending',
    result        TEXT,
    error_message TEXT,
    exit_code     INTEGER,
    environment   JSONB DEFAULT '{}',
    created_at    TIMESTAMP DEFAULT NOW(),
    finished_at   TIMESTAMP
);

-- 邮件对象表（存储发送对象扩展信息）
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

-- 邮件发送记录表
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
    token_jti     VARCHAR(255) UNIQUE NOT NULL,
    expires_at    TIMESTAMP NOT NULL,
    created_at    TIMESTAMP DEFAULT NOW()
);
