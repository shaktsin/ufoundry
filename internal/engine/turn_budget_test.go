package engine

import (
	"testing"

	"github.com/shaktsin/umcode/internal/llm"
	"github.com/shaktsin/umcode/internal/protocol"
)

func TestTurnBudgetStopsOnTokenAndCostUsage(t *testing.T) {
	limits := protocol.ExecutionLimits{MaxDurationMinutes: 5, MaxTokens: 20_000, MaxCostUSD: 1, MaxToolRounds: 200}
	b := newTurnBudget(limits)
	b.addUsage(protocol.UsageTotals{InputTokens: 12_000, OutputTokens: 8_000, CostUSD: 0.2})
	if got := b.stopReason(); got == "" {
		t.Fatal("expected token budget to stop the turn")
	}

	b = newTurnBudget(limits)
	b.addUsage(protocol.UsageTotals{InputTokens: 10, OutputTokens: 10, CostUSD: 1})
	if got := b.stopReason(); got == "" {
		t.Fatal("expected cost budget to stop the turn")
	}
}

func TestTurnBudgetDetectsIdenticalToolRounds(t *testing.T) {
	b := newTurnBudget(protocol.ExecutionLimits{MaxDurationMinutes: 5, MaxTokens: 20_000, MaxCostUSD: 1, MaxToolRounds: 200})
	calls := []llm.ToolCall{{Name: "read_file", Args: []byte(`{"path":"main.go"}`)}}
	if got := b.observeToolRound(calls, []string{"same file contents"}); got != 1 {
		t.Fatalf("first round repeat count = %d, want 1", got)
	}
	if got := b.observeToolRound(calls, []string{"same file contents"}); got != 2 {
		t.Fatalf("second round repeat count = %d, want 2", got)
	}
	if got := b.observeToolRound(calls, []string{"changed file contents"}); got != 1 {
		t.Fatalf("changed result repeat count = %d, want 1", got)
	}
}
