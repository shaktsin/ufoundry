package projects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shaktsin/ufoundry/internal/protocol"
)

// Instruction file names, in the order they are looked for. AGENT.md is ours;
// the other two let a repo set up for Codex or Claude Code work unchanged.
var instructionNames = []string{"AGENT.md", "AGENTS.md", "CLAUDE.md"}

// maxInstructionBytes caps one instruction file; the rest is dropped with a note.
const maxInstructionBytes = 32 << 10

type instructionCache struct {
	composed string
	sources  []protocol.InstructionSource
	stamps   map[string]time.Time
	nested   string
}

// InstructionsFor composes the instructions that apply to a turn: the global
// AGENT.md, then the project's, then the nearest one in the subtree the turn is
// working in (hint may be empty).
func (s *Service) InstructionsFor(ctx context.Context, p protocol.Project, hint string) (string, []protocol.InstructionSource) {
	files := s.instructionFiles(p, hint)
	key := p.ID + "\x00" + hint
	s.mu.Lock()
	c := s.cache[key]
	s.mu.Unlock()
	if c != nil && sameStamps(c.stamps, files) {
		return c.composed, c.sources
	}

	var b strings.Builder
	var sources []protocol.InstructionSource
	stamps := map[string]time.Time{}
	for _, f := range files {
		st, err := os.Stat(f.path)
		if err != nil {
			continue
		}
		stamps[f.path] = st.ModTime()
		data, err := os.ReadFile(f.path)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		src := protocol.InstructionSource{Scope: f.scope, Path: f.path, Bytes: len(data)}
		if len(data) > maxInstructionBytes {
			data = data[:maxInstructionBytes]
			src.Error = fmt.Sprintf("truncated to %d KB", maxInstructionBytes>>10)
		}
		title := map[string]string{
			"global": "# Your standing instructions (~/.ufoundry/AGENT.md)",
			"project": fmt.Sprintf("# Project instructions (%s)",
				filepath.Base(f.path)),
			"nested": fmt.Sprintf("# Instructions for %s", filepath.Dir(f.path)),
		}[f.scope]
		b.WriteString("\n" + title + "\n")
		b.Write(data)
		b.WriteString("\n")
		sources = append(sources, src)
	}
	composed := b.String()
	s.mu.Lock()
	s.cache[key] = &instructionCache{composed: composed, sources: sources, stamps: stamps}
	s.mu.Unlock()
	return composed, sources
}

type instructionFile struct {
	scope string
	path  string
}

// instructionFiles lists the candidate files, global first.
func (s *Service) instructionFiles(p protocol.Project, hint string) []instructionFile {
	var out []instructionFile
	global := s.cfg.Agents.ContextFile
	if global == "" {
		global = filepath.Join(s.cfg.Home, "AGENT.md")
	}
	out = append(out, instructionFile{scope: "global", path: global})
	if p.Root == "" {
		return out
	}
	if p.InstructionsPath != "" {
		out = append(out, instructionFile{scope: "project", path: p.InstructionsPath})
	} else if f := firstInstructionFile(p.Root); f != "" {
		out = append(out, instructionFile{scope: "project", path: f})
	}
	// The nearest instruction file above the file or folder being worked on.
	if hint != "" {
		if abs, err := Resolve(p.Root, hint); err == nil {
			dir := abs
			if st, err := os.Stat(abs); err != nil || !st.IsDir() {
				dir = filepath.Dir(abs)
			}
			for within(p.Root, dir) && dir != p.Root {
				if f := firstInstructionFile(dir); f != "" {
					out = append(out, instructionFile{scope: "nested", path: f})
					break
				}
				dir = filepath.Dir(dir)
			}
		}
	}
	return out
}

func firstInstructionFile(dir string) string {
	for _, name := range instructionNames {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func sameStamps(stamps map[string]time.Time, files []instructionFile) bool {
	seen := 0
	for _, f := range files {
		st, err := os.Stat(f.path)
		if err != nil {
			if _, had := stamps[f.path]; had {
				return false
			}
			continue
		}
		seen++
		prev, had := stamps[f.path]
		if !had || !prev.Equal(st.ModTime()) {
			return false
		}
	}
	return seen == len(stamps)
}

func (s *Service) invalidate(projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.cache {
		if strings.HasPrefix(k, projectID+"\x00") {
			delete(s.cache, k)
		}
	}
}

// InstructionsPath is where project/instructions writes: the configured file,
// an existing AGENT.md/AGENTS.md/CLAUDE.md, or a new AGENT.md in the root.
func InstructionsPath(p protocol.Project) string {
	if p.InstructionsPath != "" {
		return p.InstructionsPath
	}
	if f := firstInstructionFile(p.Root); f != "" {
		return f
	}
	return filepath.Join(p.Root, "AGENT.md")
}

// Instructions reads (and optionally writes) a project's own instruction file
// and returns the composed prompt.
func (s *Service) Instructions(ctx context.Context, p protocol.Project, content *string) (protocol.ProjectInstructionsResult, error) {
	path := InstructionsPath(p)
	if content != nil {
		if _, err := Resolve(p.Root, path); err != nil {
			return protocol.ProjectInstructionsResult{}, err
		}
		if err := os.WriteFile(path, []byte(*content), 0o644); err != nil {
			return protocol.ProjectInstructionsResult{}, err
		}
		s.invalidate(p.ID)
	}
	own := ""
	if data, err := os.ReadFile(path); err == nil {
		own = string(data)
	}
	composed, sources := s.InstructionsFor(ctx, p, "")
	return protocol.ProjectInstructionsResult{
		Composed: composed, Project: own, Path: path, Sources: sources,
	}, nil
}
