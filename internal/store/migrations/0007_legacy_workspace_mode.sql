-- Before workspace mode was selectable, every project chat was implicitly
-- assigned a Git worktree. Mark those rows for one-time runtime resolution:
-- keep chats with a saved worktree; move failed/uninitialized chats to local.
UPDATE threads SET workspace_mode = 'legacy' WHERE workspace_mode = 'worktree';
