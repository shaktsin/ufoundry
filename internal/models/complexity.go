package models

import (
	"regexp"
	"strings"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/protocol"
)

// DefaultPresets are the built-in complexity presets.
func DefaultPresets() map[protocol.Complexity]protocol.ComplexityPreset {
	return map[protocol.Complexity]protocol.ComplexityPreset{
		protocol.ComplexityQuick: {Level: protocol.ComplexityQuick, Reasoning: llm.ReasoningOff,
			MaxToolSteps: 5, MultiAgent: "never", MaxOutputTokens: 4096},
		protocol.ComplexityStandard: {Level: protocol.ComplexityStandard, Reasoning: llm.ReasoningMedium,
			MaxToolSteps: 15, MultiAgent: "teamRouteOnly", MaxOutputTokens: 16384},
		protocol.ComplexityDeep: {Level: protocol.ComplexityDeep, Reasoning: llm.ReasoningHigh,
			MaxToolSteps: 40, MultiAgent: "allowed", MaxOutputTokens: 32768},
	}
}

// Presets merges config overrides (models.complexity in config.yaml) over the defaults.
func Presets(cfg config.ModelsConfig) map[protocol.Complexity]protocol.ComplexityPreset {
	out := DefaultPresets()
	for name, o := range cfg.Complexity {
		lvl := protocol.Complexity(strings.ToLower(name))
		p, ok := out[lvl]
		if !ok {
			continue
		}
		if o.Reasoning != "" {
			p.Reasoning = o.Reasoning
		}
		if o.MaxToolSteps > 0 {
			p.MaxToolSteps = o.MaxToolSteps
		}
		if o.MultiAgent != "" {
			p.MultiAgent = o.MultiAgent
		}
		if o.MaxOutputTokens > 0 {
			p.MaxOutputTokens = o.MaxOutputTokens
		}
		out[lvl] = p
	}
	return out
}

var (
	deepWords  = regexp.MustCompile(`(?i)\b(research|analy[sz]e|analysis|investigate|design|architect|refactor|debug|root cause|compare|evaluate|plan|strategy|step[- ]by[- ]step|in depth|thorough|comprehensive|write (a|an) (report|spec|proposal)|migrate|implement)\b`)
	quickWords = regexp.MustCompile(`(?i)^(hi|hello|hey|thanks|thank you|ok|okay|yes|no|what time|what's the time|who is|what is|define|translate|convert)\b`)
)

// Classify picks a complexity for Auto from the message text. It is a cheap,
// deterministic heuristic; the engine records which level it picked so the
// user can re-run the turn at another level.
func Classify(text string, attachments int) protocol.Complexity {
	t := strings.TrimSpace(text)
	words := len(strings.Fields(t))
	switch {
	case deepWords.MatchString(t) && words >= 6:
		return protocol.ComplexityDeep
	case words > 120:
		return protocol.ComplexityDeep
	case attachments == 0 && quickWords.MatchString(t) && words <= 20:
		return protocol.ComplexityQuick
	case attachments == 0 && words <= 12 && !strings.Contains(t, "\n") &&
		strings.Count(t, ".")+strings.Count(t, "?") <= 1:
		return protocol.ComplexityQuick
	}
	return protocol.ComplexityStandard
}
