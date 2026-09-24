-- Scheduled tasks: model selection per task and the thread each task reports into.
-- New columns only; the Python app reads tasks with SELECT * and ignores them.
ALTER TABLE tasks ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN complexity TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN credential_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN thread_id TEXT NOT NULL DEFAULT '';
ALTER TABLE task_runs ADD COLUMN turn_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs(task_id, id);
