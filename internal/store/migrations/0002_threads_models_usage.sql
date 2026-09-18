-- Go engine: threads/turns/items, approvals, API key records, token usage,
-- model preferences and settings. Existing tables are left untouched so the
-- Python app can still open this database during the migration.

CREATE TABLE IF NOT EXISTS threads (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    channel TEXT NOT NULL DEFAULT 'app',
    pinned INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0,
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    complexity TEXT NOT NULL DEFAULT '',
    credential_id TEXT NOT NULL DEFAULT '',
    forked_from TEXT NOT NULL DEFAULT '',
    legacy_session_id INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_threads_updated ON threads(archived, pinned DESC, updated_at DESC);

CREATE TABLE IF NOT EXISTS turns (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    sel_provider TEXT NOT NULL DEFAULT '',
    sel_model TEXT NOT NULL DEFAULT '',
    sel_complexity TEXT NOT NULL DEFAULT '',
    sel_credential TEXT NOT NULL DEFAULT '',
    res_provider TEXT NOT NULL DEFAULT '',
    res_model TEXT NOT NULL DEFAULT '',
    res_complexity TEXT NOT NULL DEFAULT '',
    res_credential TEXT NOT NULL DEFAULT '',
    auto_picked INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_turns_thread ON turns(thread_id, started_at);

CREATE TABLE IF NOT EXISTS items (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    turn_id TEXT NOT NULL,
    seq INTEGER NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    text TEXT NOT NULL DEFAULT '',
    tool_json TEXT NOT NULL DEFAULT '',
    data_json TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_items_thread ON items(thread_id, seq);

-- Full-text index over message and tool-output text, maintained by the engine.
CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(
    item_id UNINDEXED,
    thread_id UNINDEXED,
    text,
    tokenize = 'porter unicode61'
);

CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    tool TEXT NOT NULL,
    args_json TEXT NOT NULL DEFAULT '{}',
    risk TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    action_summary TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    decided_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    decided_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status, created_at);

CREATE TABLE IF NOT EXISTS credentials (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    label TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    last4 TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    is_default INTEGER NOT NULL DEFAULT 0,
    fallback INTEGER NOT NULL DEFAULT 0,
    monthly_budget_usd REAL NOT NULL DEFAULT 0,
    hard_stop INTEGER NOT NULL DEFAULT 0,
    last_tested_at TEXT,
    last_test_ok INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_credentials_provider ON credentials(provider);

CREATE TABLE IF NOT EXISTS llm_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TEXT NOT NULL,
    credential_id TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    thread_id TEXT NOT NULL DEFAULT '',
    turn_id TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'chat',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    cached_input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'ok',
    estimated INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_usage_credential ON llm_usage(credential_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_thread ON llm_usage(thread_id);
CREATE INDEX IF NOT EXISTS idx_usage_created ON llm_usage(created_at);

-- Per-model user preferences: hidden in the picker, price overrides.
CREATE TABLE IF NOT EXISTS model_prefs (
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    hidden INTEGER NOT NULL DEFAULT 0,
    price_override INTEGER NOT NULL DEFAULT 0,
    input_per_mtok REAL NOT NULL DEFAULT 0,
    cached_input_per_mtok REAL NOT NULL DEFAULT 0,
    output_per_mtok REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (provider, model)
);

-- Model lists fetched from provider APIs (cached ~24h).
CREATE TABLE IF NOT EXISTS model_cache (
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    fetched_at TEXT NOT NULL,
    PRIMARY KEY (provider, model)
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Backfill: every Python session becomes a thread with one legacy turn that
-- holds its messages, so old chats appear in history.
INSERT OR IGNORE INTO threads (id, title, channel, legacy_session_id, created_at, updated_at)
SELECT 'legacy-s' || s.id,
       COALESCE(substr((SELECT m.content FROM messages m
                        WHERE m.session_id = s.id AND m.role = 'user'
                        ORDER BY m.id LIMIT 1), 1, 60), 'Chat ' || s.id),
       s.channel,
       s.id,
       s.created_at,
       COALESCE((SELECT MAX(m.created_at) FROM messages m WHERE m.session_id = s.id), s.created_at)
FROM sessions s;

INSERT OR IGNORE INTO turns (id, thread_id, status, started_at, finished_at)
SELECT 'legacy-t' || s.id, 'legacy-s' || s.id, 'completed', s.created_at, s.created_at
FROM sessions s;

INSERT OR IGNORE INTO items (id, thread_id, turn_id, seq, kind, status, text, created_at)
SELECT 'legacy-m' || m.id,
       'legacy-s' || m.session_id,
       'legacy-t' || m.session_id,
       m.id,
       CASE WHEN m.role = 'user' THEN 'userMessage' ELSE 'agentMessage' END,
       'completed',
       m.content,
       m.created_at
FROM messages m
WHERE m.role IN ('user', 'assistant') AND EXISTS (SELECT 1 FROM sessions s WHERE s.id = m.session_id);

INSERT INTO items_fts (item_id, thread_id, text)
SELECT id, thread_id, text FROM items WHERE id LIKE 'legacy-m%' AND text != '';
