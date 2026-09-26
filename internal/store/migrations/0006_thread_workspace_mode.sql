-- Older project chats were always created in UMCode-managed Git workspaces.
-- Preserve that behavior for them; new chats explicitly default to local.
ALTER TABLE threads ADD COLUMN workspace_mode TEXT NOT NULL DEFAULT 'worktree';
