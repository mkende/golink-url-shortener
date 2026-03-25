CREATE TABLE IF NOT EXISTS links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    name_lower TEXT NOT NULL,
    target TEXT NOT NULL,
    owner_email TEXT NOT NULL,
    is_advanced BOOLEAN NOT NULL DEFAULT FALSE,
    require_auth BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME,
    use_count INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_links_name_lower ON links(name_lower);
CREATE INDEX IF NOT EXISTS idx_links_owner_email ON links(owner_email);
CREATE INDEX IF NOT EXISTS idx_links_use_count ON links(use_count DESC);
CREATE INDEX IF NOT EXISTS idx_links_created_at ON links(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_links_last_used_at ON links(last_used_at DESC);

CREATE TABLE IF NOT EXISTS link_shares (
    link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    shared_with_email TEXT NOT NULL,
    PRIMARY KEY (link_id, shared_with_email)
);

CREATE INDEX IF NOT EXISTS idx_link_shares_email ON link_shares(shared_with_email);

CREATE TABLE IF NOT EXISTS users (
    email TEXT PRIMARY KEY,
    display_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS groups (
    name TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'oidc',
    PRIMARY KEY (name, source)
);

CREATE TABLE IF NOT EXISTS group_members (
    group_name TEXT NOT NULL,
    group_source TEXT NOT NULL,
    user_email TEXT NOT NULL,
    PRIMARY KEY (group_name, group_source, user_email),
    FOREIGN KEY (group_name, group_source) REFERENCES groups(name, source) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_group_members_email ON group_members(user_email);

CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
