package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/shaktsin/umcode/internal/protocol"
	"github.com/shaktsin/umcode/internal/store"
)

// project loads a project, turning a missing id into a protocol error.
func (e *Engine) project(ctx context.Context, id string) (protocol.Project, error) {
	if id == "" {
		return protocol.Project{}, protocol.Errorf(protocol.CodeInvalidParams, "projectId is required")
	}
	p, err := e.Projects.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return p, protocol.Errorf(protocol.CodeNotFound, "project %s does not exist", id)
	}
	return p, err
}

// CreateProject registers a folder as a project.
func (e *Engine) CreateProject(ctx context.Context, p protocol.ProjectCreateParams) (protocol.Project, error) {
	proj, err := e.Projects.Create(ctx, p)
	if err != nil {
		return proj, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	_ = e.Store.Audit(ctx, "project.create", map[string]any{"id": proj.ID, "root": proj.Root})
	e.Bus.PublishAdmin(protocol.NotifyProjectUpdated, protocol.ProjectEvent{Project: proj})
	return proj, nil
}

// OpenProject marks a project as most recently used.
func (e *Engine) OpenProject(ctx context.Context, id string) (protocol.Project, error) {
	if _, err := e.project(ctx, id); err != nil {
		return protocol.Project{}, err
	}
	return e.Projects.Open(ctx, id)
}

// UpdateProject changes a project's name, settings or tools.
func (e *Engine) UpdateProject(ctx context.Context, p protocol.ProjectUpdateParams) (protocol.Project, error) {
	if _, err := e.project(ctx, p.ProjectID); err != nil {
		return protocol.Project{}, err
	}
	proj, err := e.Projects.Update(ctx, p)
	if err != nil {
		return proj, err
	}
	e.Bus.PublishAdmin(protocol.NotifyProjectUpdated, protocol.ProjectEvent{Project: proj})
	return proj, nil
}

// DeleteProject forgets a project; its folder is left alone and its chats stay.
func (e *Engine) DeleteProject(ctx context.Context, id string) error {
	proj, err := e.project(ctx, id)
	if err != nil {
		return err
	}
	if err := e.Projects.Delete(ctx, id); err != nil {
		return err
	}
	_ = e.Store.Audit(ctx, "project.delete", map[string]any{"id": id, "root": proj.Root})
	e.Bus.PublishAdmin(protocol.NotifyProjectUpdated, protocol.ProjectEvent{Project: proj, Deleted: true})
	return nil
}

// ProjectInstructions reads, and optionally writes, a project's UMCODE.md.
func (e *Engine) ProjectInstructions(ctx context.Context, p protocol.ProjectInstructionsParams) (protocol.ProjectInstructionsResult, error) {
	proj, err := e.project(ctx, p.ProjectID)
	if err != nil {
		return protocol.ProjectInstructionsResult{}, err
	}
	res, err := e.Projects.Instructions(ctx, proj, p.Content)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	if p.Content != nil {
		_ = e.Store.Audit(ctx, "project.instructions", map[string]any{"id": proj.ID, "path": res.Path})
	}
	return res, nil
}

// ScanProjectInstructions returns a read-only UMCODE.md draft based on project files.
func (e *Engine) ScanProjectInstructions(ctx context.Context, p protocol.ProjectIDParams) (protocol.ProjectInstructionDraft, error) {
	proj, err := e.project(ctx, p.ProjectID)
	if err != nil {
		return protocol.ProjectInstructionDraft{}, err
	}
	draft, err := e.Projects.DraftInstructions(proj)
	if err != nil {
		return draft, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	_ = e.Store.Audit(ctx, "project.instructions.scan", map[string]any{"id": proj.ID, "files": len(draft.ScannedFiles)})
	return draft, nil
}

// ProjectFiles lists a directory inside a project.
func (e *Engine) ProjectFiles(ctx context.Context, p protocol.ProjectFilesParams) (protocol.ProjectFilesResult, error) {
	proj, err := e.project(ctx, p.ProjectID)
	if err != nil {
		return protocol.ProjectFilesResult{}, err
	}
	if proj, err = e.projectTaskRoot(ctx, proj, p.ThreadID); err != nil {
		return protocol.ProjectFilesResult{}, err
	}
	res, err := e.Projects.Files(ctx, proj, p)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	if res.Entries == nil {
		res.Entries = []protocol.FileEntry{}
	}
	return res, nil
}

// ProjectReadFile returns one file's contents.
func (e *Engine) ProjectReadFile(ctx context.Context, p protocol.ProjectReadFileParams) (protocol.ProjectReadFileResult, error) {
	proj, err := e.project(ctx, p.ProjectID)
	if err != nil {
		return protocol.ProjectReadFileResult{}, err
	}
	if proj, err = e.projectTaskRoot(ctx, proj, p.ThreadID); err != nil {
		return protocol.ProjectReadFileResult{}, err
	}
	res, err := e.Projects.ReadFile(ctx, proj, p)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	return res, nil
}

func (e *Engine) ProjectReadArtifact(ctx context.Context, p protocol.ProjectReadArtifactParams) (protocol.ProjectReadArtifactResult, error) {
	proj, err := e.project(ctx, p.ProjectID)
	if err != nil {
		return protocol.ProjectReadArtifactResult{}, err
	}
	if proj, err = e.projectTaskRoot(ctx, proj, p.ThreadID); err != nil {
		return protocol.ProjectReadArtifactResult{}, err
	}
	res, err := e.Projects.ReadArtifact(ctx, proj, p)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	return res, nil
}

func (e *Engine) projectTaskRoot(ctx context.Context, proj protocol.Project, threadID string) (protocol.Project, error) {
	if threadID == "" {
		return proj, nil
	}
	th, err := e.Store.GetThread(ctx, threadID)
	if err != nil {
		return proj, err
	}
	th, err = e.resolveLegacyWorkspaceMode(ctx, th)
	if err != nil {
		return proj, err
	}
	if th.ProjectID != proj.ID {
		return proj, protocol.Errorf(protocol.CodeInvalidParams, "chat does not belong to this project")
	}
	if th.WorkspaceMode != "worktree" {
		return proj, nil
	}
	ws, err := e.Worktrees.Ensure(ctx, th.ID, proj.Root)
	if err != nil {
		return proj, err
	}
	proj.Root = ws.Path
	return proj, nil
}

// ProjectDiff reports what the agent changed.
func (e *Engine) ProjectDiff(ctx context.Context, p protocol.ProjectDiffParams) (protocol.ProjectDiffResult, error) {
	if p.ProjectID == "" && p.TurnID == "" {
		return protocol.ProjectDiffResult{}, protocol.Errorf(protocol.CodeInvalidParams, "projectId or turnId is required")
	}
	proj := protocol.Project{}
	var threadID string
	if p.TurnID != "" {
		changes, err := e.Store.ListFileChanges(ctx, p.ProjectID, p.TurnID, "", 1)
		if err != nil {
			return protocol.ProjectDiffResult{}, err
		}
		if len(changes) > 0 {
			threadID = changes[0].ThreadID
			if p.ProjectID == "" {
				p.ProjectID = changes[0].ProjectID
			}
		}
	}
	if p.ProjectID != "" {
		var err error
		if proj, err = e.project(ctx, p.ProjectID); err != nil {
			return protocol.ProjectDiffResult{}, err
		}
	} else {
		changes, err := e.Store.ListFileChanges(ctx, "", p.TurnID, "", 1)
		if err != nil || len(changes) == 0 {
			return protocol.ProjectDiffResult{Files: []protocol.FileChangeData{}}, err
		}
		if proj, err = e.project(ctx, changes[0].ProjectID); err != nil {
			return protocol.ProjectDiffResult{}, err
		}
	}
	if p.TurnID != "" && threadID != "" {
		th, err := e.Store.GetThread(ctx, threadID)
		if err != nil {
			return protocol.ProjectDiffResult{}, err
		}
		th, err = e.resolveLegacyWorkspaceMode(ctx, th)
		if err != nil {
			return protocol.ProjectDiffResult{}, err
		}
		if th.WorkspaceMode == "worktree" {
			workspace, err := e.Worktrees.Ensure(ctx, threadID, proj.Root)
			if err != nil {
				return protocol.ProjectDiffResult{}, err
			}
			proj.Root = workspace.Path
		}
	}
	res, err := e.Projects.Diff(ctx, proj, p)
	if res.Files == nil {
		res.Files = []protocol.FileChangeData{}
	}
	return res, err
}

// RevertTurn puts the files a turn changed back the way it found them.
func (e *Engine) RevertTurn(ctx context.Context, p protocol.ProjectRevertTurnParams) (protocol.ProjectRevertTurnResult, error) {
	if p.TurnID == "" {
		return protocol.ProjectRevertTurnResult{}, protocol.Errorf(protocol.CodeInvalidParams, "turnId is required")
	}
	e.mu.Lock()
	_, running := e.activeTurns[p.TurnID]
	e.mu.Unlock()
	if running {
		return protocol.ProjectRevertTurnResult{}, protocol.Errorf(protocol.CodeConflict, "turn %s is still running; stop it first", p.TurnID)
	}
	changes, err := e.Store.ListFileChanges(ctx, "", p.TurnID, "", 1)
	if err != nil {
		return protocol.ProjectRevertTurnResult{}, err
	}
	roots := map[string]string{}
	if len(changes) > 0 {
		proj, err := e.project(ctx, changes[0].ProjectID)
		if err != nil {
			return protocol.ProjectRevertTurnResult{}, err
		}
		th, err := e.Store.GetThread(ctx, changes[0].ThreadID)
		if err != nil {
			return protocol.ProjectRevertTurnResult{}, err
		}
		th, err = e.resolveLegacyWorkspaceMode(ctx, th)
		if err != nil {
			return protocol.ProjectRevertTurnResult{}, err
		}
		if th.WorkspaceMode == "worktree" {
			workspace, err := e.Worktrees.Ensure(ctx, changes[0].ThreadID, proj.Root)
			if err != nil {
				return protocol.ProjectRevertTurnResult{}, err
			}
			roots[proj.ID] = workspace.Path
		}
	}
	res, err := e.Projects.RevertTurnAt(ctx, p.TurnID, p.Paths, roots)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	_ = e.Store.Audit(ctx, "project.revertTurn", map[string]any{"turn": p.TurnID, "files": res.Reverted})
	if res.Reverted == nil {
		res.Reverted = []string{}
	}
	return res, nil
}

// KeepTaskChanges applies a stopped chat's workspace changes to its project
// checkout without creating a commit or pushing anywhere.
func (e *Engine) KeepTaskChanges(ctx context.Context, p protocol.TaskWorkspaceParams) (protocol.TaskWorkspaceResult, error) {
	th, proj, err := e.taskWorkspaceProject(ctx, p.ThreadID)
	if err != nil {
		return protocol.TaskWorkspaceResult{}, err
	}
	if th.WorkspaceMode != "worktree" {
		return protocol.TaskWorkspaceResult{}, protocol.Errorf(protocol.CodeInvalidParams, "this chat edits the project folder directly; there is no isolated workspace to keep")
	}
	e.mu.Lock()
	_, running := e.threadTurns[th.ID]
	e.mu.Unlock()
	if running {
		return protocol.TaskWorkspaceResult{}, protocol.Errorf(protocol.CodeConflict, "stop the chat's running turn before keeping changes")
	}
	n, err := e.Worktrees.Keep(ctx, th.ID, proj.Root)
	if err != nil {
		return protocol.TaskWorkspaceResult{}, protocol.Errorf(protocol.CodeConflict, "%v", err)
	}
	message := "No task changes to apply."
	if n > 0 {
		message = fmt.Sprintf("Applied %d changed files to the project checkout. No commit was created.", n)
	}
	_ = e.Store.Audit(ctx, "project.keepTaskChanges", map[string]any{"thread": th.ID, "files": n})
	return protocol.TaskWorkspaceResult{ChangedFiles: n, Message: message}, nil
}

// DiscardTaskWorkspace removes a stopped chat's isolated working copy only.
func (e *Engine) DiscardTaskWorkspace(ctx context.Context, p protocol.TaskWorkspaceParams) (protocol.TaskWorkspaceResult, error) {
	th, proj, err := e.taskWorkspaceProject(ctx, p.ThreadID)
	if err != nil {
		return protocol.TaskWorkspaceResult{}, err
	}
	if th.WorkspaceMode != "worktree" {
		return protocol.TaskWorkspaceResult{}, protocol.Errorf(protocol.CodeInvalidParams, "this chat edits the project folder directly; there is no isolated workspace to discard")
	}
	e.mu.Lock()
	_, running := e.threadTurns[th.ID]
	e.mu.Unlock()
	if running {
		return protocol.TaskWorkspaceResult{}, protocol.Errorf(protocol.CodeConflict, "stop the chat's running turn before discarding its workspace")
	}
	if err := e.Worktrees.Discard(ctx, th.ID, proj.Root); err != nil {
		return protocol.TaskWorkspaceResult{}, protocol.Errorf(protocol.CodeConflict, "%v", err)
	}
	_ = e.Store.Audit(ctx, "project.discardTaskWorkspace", map[string]any{"thread": th.ID})
	return protocol.TaskWorkspaceResult{Message: "Task workspace discarded. The project checkout was not changed."}, nil
}

func (e *Engine) taskWorkspaceProject(ctx context.Context, threadID string) (protocol.Thread, protocol.Project, error) {
	if threadID == "" {
		return protocol.Thread{}, protocol.Project{}, protocol.Errorf(protocol.CodeInvalidParams, "threadId is required")
	}
	th, err := e.Store.GetThread(ctx, threadID)
	if err != nil {
		return protocol.Thread{}, protocol.Project{}, err
	}
	if th.ProjectID == "" {
		return th, protocol.Project{}, protocol.Errorf(protocol.CodeInvalidParams, "this chat is not attached to a project")
	}
	proj, err := e.project(ctx, th.ProjectID)
	if err != nil {
		return th, proj, err
	}
	if proj.Missing {
		return th, proj, protocol.Errorf(protocol.CodeNotFound, "project folder %s is no longer available", proj.Root)
	}
	return th, proj, nil
}
