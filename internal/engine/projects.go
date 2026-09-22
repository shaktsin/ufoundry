package engine

import (
	"context"
	"errors"

	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
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

// ProjectInstructions reads, and optionally writes, a project's AGENT.md.
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

// ProjectFiles lists a directory inside a project.
func (e *Engine) ProjectFiles(ctx context.Context, p protocol.ProjectFilesParams) (protocol.ProjectFilesResult, error) {
	proj, err := e.project(ctx, p.ProjectID)
	if err != nil {
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
	res, err := e.Projects.ReadFile(ctx, proj, p)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	return res, nil
}

// ProjectDiff reports what the agent changed.
func (e *Engine) ProjectDiff(ctx context.Context, p protocol.ProjectDiffParams) (protocol.ProjectDiffResult, error) {
	if p.ProjectID == "" && p.TurnID == "" {
		return protocol.ProjectDiffResult{}, protocol.Errorf(protocol.CodeInvalidParams, "projectId or turnId is required")
	}
	proj := protocol.Project{}
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
	res, err := e.Projects.RevertTurn(ctx, p.TurnID, p.Paths)
	if err != nil {
		return res, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	_ = e.Store.Audit(ctx, "project.revertTurn", map[string]any{"turn": p.TurnID, "files": res.Reverted})
	if res.Reverted == nil {
		res.Reverted = []string{}
	}
	return res, nil
}
