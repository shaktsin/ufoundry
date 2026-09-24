-- What the router has learned about each key and model: cooldowns after a rate
-- limit or outage, and a rolling count of how requests have gone. Keeping this
-- in SQLite means a restart does not hammer a key that is rate-limiting us.
CREATE TABLE IF NOT EXISTS model_health (
    provider       TEXT NOT NULL,
    model          TEXT NOT NULL,
    credential_id  TEXT NOT NULL,
    cooldown_until TEXT NOT NULL DEFAULT '',
    last_status    TEXT NOT NULL DEFAULT '',
    last_error     TEXT NOT NULL DEFAULT '',
    ok_count       INTEGER NOT NULL DEFAULT 0,
    err_count      INTEGER NOT NULL DEFAULT 0,
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (provider, model, credential_id)
);

-- Which routes a turn actually used, in order, so the UI can say why it
-- switched and Usage can attribute tokens to the key that served them.
ALTER TABLE turns ADD COLUMN route_trail TEXT NOT NULL DEFAULT '';
