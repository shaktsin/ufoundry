package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/shaktsin/umcode/internal/llm"
	"github.com/shaktsin/umcode/internal/protocol"
)

func TestHistoryMessagesUsesMostRecentCompactionAndKeepsLaterTurns(t *testing.T) {
	now := time.Now()
	items := []protocol.Item{
		{TurnID: "one", Kind: protocol.ItemUserMessage, Status: protocol.ItemCompleted, Text: "old request"},
		{TurnID: "one", Kind: protocol.ItemAgentMessage, Status: protocol.ItemCompleted, Text: "old answer"},
		{TurnID: "one", Kind: protocol.ItemContextCompaction, Status: protocol.ItemCompleted, Text: "summary of old request"},
		{TurnID: "two", Kind: protocol.ItemUserMessage, Status: protocol.ItemCompleted, Text: "follow-up"},
		{TurnID: "two", Kind: protocol.ItemAgentMessage, Status: protocol.ItemCompleted, Text: "follow-up answer"},
	}
	for i := range items {
		items[i].CreatedAt = now
	}
	messages := historyMessages(items, "", 0)
	if len(messages) != 2 || messages[0].JoinedText() != "Earlier conversation summary (the full transcript remains visible in UMCode):\nsummary of old request\n\nfollow-up" || messages[1].JoinedText() != "follow-up answer" {
		t.Fatalf("compacted messages = %#v", messages)
	}
}

func TestCompactionTranscriptBoundsLongConversations(t *testing.T) {
	long := compactionTranscript([]llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{{Type: "text", Text: strings.Repeat("context ", maxCompactionInput)}}}})
	if len([]rune(long)) > maxCompactionInput+200 || !strings.Contains(long, "Middle of conversation omitted") {
		t.Fatalf("bounded transcript has %d runes or is missing omission marker", len([]rune(long)))
	}
}
