// Package projects manages the folders the agent is allowed to work in. A
// project is the sandbox boundary: file and shell tools resolve every path
// against its root and refuse anything outside it.
package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
)

// Service is the project registry.
type Service struct {
	st  *store.Store
	cfg *config.Config

	mu    sync.Mutex
	cache map[string]*instructionCache
}

// New returns a project service.
func New(st *store.Store, cfg *config.Config) *Service {
	return &Service{st: st, cfg: cfg, cache: map[string]*instructionCache{}}
}

// ErrNoProject is returned when a chat without a project tries to change files.
var ErrNoProject = errors.New("this chat is not attached to a project, so it cannot read or change files; open a project first")

// ExpandPath expands ~ and makes a path absolute and clean.
func ExpandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("path is empty")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// Create registers a folder as a project. The folder must exist; a project for
// the same folder (after resolving symlinks) is returned as an error.
func (s *Service) Create(ctx context.Context, p protocol.ProjectCreateParams) (protocol.Project, error) {
	root, err := ExpandPath(p.Root)
	if err != nil {
		return protocol.Project{}, err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	st, err := os.Stat(root)
	if err != nil {
		return protocol.Project{}, fmt.Errorf("%s cannot be opened: %w", root, err)
	}
	if !st.IsDir() {
		return protocol.Project{}, fmt.Errorf("%s is a file, not a folder", root)
	}
	if home, err := os.UserHomeDir(); err == nil && (root == home || root == "/") {
		return protocol.Project{}, fmt.Errorf("%s is too broad to be a project; pick the folder you actually work in", root)
	}
	if strings.HasPrefix(root+string(filepath.Separator), s.cfg.Home+string(filepath.Separator)) {
		return protocol.Project{}, fmt.Errorf("%s is inside UFoundry's own data folder", root)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = filepath.Base(root)
	}
	rec, err := s.st.CreateProject(ctx, protocol.Project{
		Name: name, Root: root, Settings: p.Settings, Tools: p.Tools,
	})
	if errors.Is(err, store.ErrDuplicate) {
		existing, gerr := s.st.ProjectByRoot(ctx, root)
		if gerr == nil {
			return existing, fmt.Errorf("%s is already a project (%s)", root, existing.Name)
		}
	}
	if err != nil {
		return rec, err
	}
	return s.decorate(rec), nil
}

// Get returns one project with its live folder and git state.
func (s *Service) Get(ctx context.Context, id string) (protocol.Project, error) {
	p, err := s.st.GetProject(ctx, id)
	if err != nil {
		return p, err
	}
	return s.decorate(p), nil
}

// List returns the known projects, most recently opened first.
func (s *Service) List(ctx context.Context, includeArchived bool) ([]protocol.Project, error) {
	list, err := s.st.ListProjects(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i] = s.decorate(list[i])
	}
	return list, nil
}

// Open marks a project as the most recently used one.
func (s *Service) Open(ctx context.Context, id string) (protocol.Project, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return p, err
	}
	if err := s.st.TouchProject(ctx, id); err != nil {
		return p, err
	}
	return p, nil
}

// Update changes a project's name, settings, tools or archived flag.
func (s *Service) Update(ctx context.Context, p protocol.ProjectUpdateParams) (protocol.Project, error) {
	if p.InstructionsPath != nil && *p.InstructionsPath != "" {
		cur, err := s.st.GetProject(ctx, p.ProjectID)
		if err != nil {
			return cur, err
		}
		abs, err := Resolve(cur.Root, *p.InstructionsPath)
		if err != nil {
			return cur, fmt.Errorf("instructions file: %w", err)
		}
		rel := abs
		*p.InstructionsPath = rel
	}
	updated, err := s.st.UpdateProject(ctx, p)
	if err != nil {
		return updated, err
	}
	s.invalidate(updated.ID)
	return s.decorate(updated), nil
}

// Delete forgets a project. The folder is left alone.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.st.DeleteProject(ctx, id); err != nil {
		return err
	}
	s.invalidate(id)
	return nil
}

// decorate adds the state that is not in the database.
func (s *Service) decorate(p protocol.Project) protocol.Project {
	if st, err := os.Stat(p.Root); err != nil || !st.IsDir() {
		p.Missing = true
		return p
	}
	if v := gitInfo(p.Root); v != nil {
		p.VCS = v
	}
	return p
}

// Resolve joins rel to root and refuses anything that escapes the project,
// including through symlinks. It returns an absolute path.
func Resolve(root, rel string) (string, error) {
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return root, nil
	}
	if strings.HasPrefix(rel, "~") {
		return "", fmt.Errorf("%s is outside the project", rel)
	}
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)
	if !within(root, target) {
		return "", fmt.Errorf("%s is outside the project folder (%s)", rel, root)
	}
	// Resolve symlinks on the deepest part that exists, so a link cannot point out.
	probe := target
	for {
		if resolved, err := filepath.EvalSymlinks(probe); err == nil {
			if !within(root, resolved) {
				return "", fmt.Errorf("%s resolves to %s, outside the project folder", rel, resolved)
			}
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	return target, nil
}

func within(root, target string) bool {
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

// Rel returns the slash-separated path of abs inside the project.
func Rel(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}
