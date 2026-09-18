// Package tools holds the tool registry and the built-in tools (files, shell),
// with workspace access control.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Risk tiers.
type Risk string

const (
	RiskGreen  Risk = "green"
	RiskYellow Risk = "yellow"
	RiskRed    Risk = "red"
)

// Tool is something the agent can call.
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	// Assess returns the risk of a specific call and a one-line human summary.
	Assess(args json.RawMessage) (Risk, string)
	Call(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry maps tool names to tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Add registers a tool.
func (r *Registry) Add(t Tool) { r.tools[t.Name()] = t }

// Get finds a tool by its canonical name (e.g. "shell.run") or wire name ("shell__run").
func (r *Registry) Get(name string) (Tool, bool) {
	if t, ok := r.tools[name]; ok {
		return t, true
	}
	t, ok := r.tools[FromWire(name)]
	return t, ok
}

// All returns tools sorted by name.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ToWire converts "shell.run" to "shell__run": provider APIs reject dots in names.
func ToWire(name string) string { return strings.ReplaceAll(name, ".", "__") }

// FromWire reverses ToWire.
func FromWire(name string) string { return strings.ReplaceAll(name, "__", ".") }

func decode[T any](args json.RawMessage) (T, error) {
	var v T
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	if err := json.Unmarshal(args, &v); err != nil {
		return v, fmt.Errorf("invalid arguments: %w", err)
	}
	return v, nil
}
