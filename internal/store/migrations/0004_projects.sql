-- Projects: the folder a chat works in, and the sandbox boundary for its tools.
CREATE TABLE IF NOT EXISTS projects (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    root              TEXT NOT NULL UNIQUE,
    instructions_path TEXT NOT NULL DEFAULT '',
    provider          TEXT NOT NULL DEFAULT '',
    model             TEXT NOT NULL DEFAULT '',
    complexity        TEXT NOT NULL DEFAULT '',
    credential_id     TEXT NOT NULL DEFAULT '',
    tools             TEXT NOT NULL DEFAULT '{}',
    archived          INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    last_opened_at    TEXT NOT NULL
);

-- Chats belong to a project. Empty means no project: read-only for file tools.
ALTER TABLE threads ADD COLUMN project_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_threads_project ON threads(project_id, updated_at);

-- Decisions the user asked to remember, e.g. "always allow `npm test` here".
CREATE TABLE IF NOT EXISTS project_approvals (
    project_id TEXT NOT NULL,
    tool       TEXT NOT NULL,
    signature  TEXT NOT NULL,
    decision   TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (project_id, tool, signature)
);

-- Every file the agent created, changed or deleted, with enough of the old
-- content to undo it (NULL when the file was too large to keep).
CREATE TABLE IF NOT EXISTS file_changes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT NOT NULL,
    thread_id   TEXT NOT NULL DEFAULT '',
    turn_id     TEXT NOT NULL DEFAULT '',
    item_id     TEXT NOT NULL DEFAULT '',
    path        TEXT NOT NULL,
    action      TEXT NOT NULL,
    before_blob TEXT,
    after_hash  TEXT NOT NULL DEFAULT '',
    additions   INTEGER NOT NULL DEFAULT 0,
    deletions   INTEGER NOT NULL DEFAULT 0,
    revertable  INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_changes_turn ON file_changes(turn_id, id);
CREATE INDEX IF NOT EXISTS idx_file_changes_project ON file_changes(project_id, id);
