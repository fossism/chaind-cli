package store

const SchemaSQL = `
-- PRAGMAS are handled in sqlite.go

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    platform TEXT,
    platform_id TEXT,
    room_id TEXT,
    author_id TEXT,
    text TEXT,
    timestamp DATETIME,
    root_id TEXT,
    parent_id TEXT,
    read BOOLEAN,
    edited BOOLEAN,
    deleted BOOLEAN,
    UNIQUE(platform, platform_id)
);

CREATE TABLE IF NOT EXISTS rooms (
    id TEXT PRIMARY KEY,
    platform TEXT,
    platform_id TEXT,
    alias TEXT,
    name TEXT,
    type TEXT,
    UNIQUE(platform, platform_id)
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    platform TEXT,
    platform_id TEXT,
    username TEXT,
    display_name TEXT,
    is_bot BOOLEAN,
    UNIQUE(platform, platform_id)
);

CREATE TABLE IF NOT EXISTS sync_state (
    platform TEXT,
    key TEXT,
    value TEXT,
    PRIMARY KEY (platform, key)
);

CREATE TABLE IF NOT EXISTS outbox (
    id TEXT PRIMARY KEY,
    platform TEXT,
    room_id TEXT,
    content TEXT,
    scheduled_at DATETIME,
    token_name TEXT
);

CREATE TABLE IF NOT EXISTS modlog (
    id TEXT PRIMARY KEY,
    action TEXT,
    target TEXT,
    operator TEXT,
    reason TEXT,
    dry_run BOOLEAN,
    timestamp DATETIME
);

CREATE TABLE IF NOT EXISTS approval_queue (
    id TEXT PRIMARY KEY,
    action_type TEXT,
    platform TEXT,
    room_id TEXT,
    payload TEXT,
    created_at DATETIME
);

CREATE TABLE IF NOT EXISTS access_log (
    token_name TEXT,
    operation TEXT,
    rooms TEXT,
    msg_count INTEGER,
    pid INTEGER,
    ts DATETIME
);

CREATE TABLE IF NOT EXISTS tokens (
    name TEXT PRIMARY KEY,
    tier INTEGER,
    rooms TEXT,
    pii_scrub TEXT,
    expires DATETIME,
    revoked BOOLEAN
);

CREATE TABLE IF NOT EXISTS optout (
    user_id TEXT,
    platform TEXT,
    opted_at DATETIME,
    PRIMARY KEY (user_id, platform)
);

-- FTS5 Virtual Table for Search
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text,
    content='messages',
    content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
END;

CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.rowid, old.text);
END;

CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.rowid, old.text);
  INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
END;

CREATE TABLE IF NOT EXISTS sync_cursors (
    platform TEXT,
    room_id TEXT,
    cursor_timestamp INTEGER,
    PRIMARY KEY (platform, room_id)
);
`
