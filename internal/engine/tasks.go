package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
)

// runTask executes a scheduled task as a turn in the task's own thread and
// waits for it to finish. Approvals go to admin clients as usual.
func (e *Engine) runTask(ctx context.Context, t protocol.Task) (string, string, error) {
	sctx := context.WithoutCancel(ctx)
	threadID := t.ThreadID
	if threadID != "" {
		if _, err := e.Store.GetThread(sctx, threadID); errors.Is(err, store.ErrNotFound) {
			threadID = ""
		}
	}
	if threadID == "" {
		th, err := e.Store.CreateThread(sctx, protocol.Thread{Title: "Task: " + t.Name, Channel: "task", Settings: t.Settings})
		if err != nil {
			return "", "", err
		}
		threadID = th.ID
		if err := e.Store.SetTaskThread(sctx, t.ID, threadID); err != nil {
			return "", "", err
		}
	}
	text := fmt.Sprintf("Scheduled task #%d %q is due now.\nTask instruction: %s\nDo it now and reply with the final result.", t.ID, t.Name, t.Prompt)
	done := make(chan protocol.Turn, 1)
	turn, err := e.startTurn(sctx, turnRequest{
		TurnStartParams: protocol.TurnStartParams{ThreadID: threadID, Text: text, Override: t.Settings},
		wait:            done,
	})
	if err != nil {
		return "", "", err
	}
	var final protocol.Turn
	select {
	case final = <-done:
	case <-ctx.Done():
		_ = e.InterruptTurn(turn.ID)
		final = <-done
	}
	result := e.lastReply(sctx, threadID, turn.ID)
	switch final.Status {
	case protocol.TurnCompleted:
		return turn.ID, result, nil
	case protocol.TurnInterrupted:
		return turn.ID, result, errors.New("interrupted")
	default:
		return turn.ID, result, errors.New(strings.TrimSpace("turn failed: " + final.Error))
	}
}

// lastReply returns the text of the last completed agent message in a turn.
func (e *Engine) lastReply(ctx context.Context, threadID, turnID string) string {
	items, err := e.Store.ListItems(ctx, threadID, 0)
	if err != nil {
		return ""
	}
	out := ""
	for _, it := range items {
		if it.TurnID == turnID && it.Kind == protocol.ItemAgentMessage && it.Status == protocol.ItemCompleted {
			out = it.Text
		}
	}
	return out
}
