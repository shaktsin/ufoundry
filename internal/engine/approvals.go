package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
	"github.com/shaktsin/ufoundry/internal/tools"
)

// ErrApprovalExpired is returned when nobody answered in time.
var ErrApprovalExpired = errors.New("approval request expired")

// requestApproval persists an approval, broadcasts it to admin clients and
// blocks until one answers, it expires, or the turn is cancelled.
func (e *Engine) requestApproval(ctx, sctx context.Context, turn protocol.Turn, it protocol.Item, tool string,
	args json.RawMessage, risk tools.Risk, reason, summary string) (bool, error) {
	timeout := time.Duration(e.Cfg.Policy.ApprovalTimeoutMinutes) * time.Minute
	now := time.Now().UTC()
	a := protocol.Approval{
		ID: store.NewID("apr"), ThreadID: turn.ThreadID, TurnID: turn.ID, ItemID: it.ID, Tool: tool, Args: args,
		Risk: string(risk), Reason: reason, ActionSummary: summary, Status: "pending",
		CreatedAt: now, ExpiresAt: now.Add(timeout),
	}
	if err := e.Store.CreateApproval(sctx, a); err != nil {
		return false, err
	}
	ch := make(chan bool, 1)
	e.mu.Lock()
	e.approvals[a.ID] = ch
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.approvals, a.ID)
		e.mu.Unlock()
	}()

	// Admin clients get the request; clients following the thread see it too.
	ev := protocol.ApprovalEvent{Approval: a}
	e.Bus.PublishAdmin(protocol.NotifyApprovalRequest, ev)
	_ = e.Store.Audit(sctx, "approval.request", map[string]any{"id": a.ID, "tool": tool, "risk": risk, "summary": summary})

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ok := <-ch:
		return ok, nil
	case <-timer.C:
		if err := e.Store.DecideApproval(sctx, a.ID, "expired", "timeout"); err == nil {
			a.Status = "expired"
			e.Bus.PublishAdmin(protocol.NotifyApprovalResolved, protocol.ApprovalEvent{Approval: a})
			return false, ErrApprovalExpired
		}
		// Someone answered at the last moment.
		select {
		case ok := <-ch:
			return ok, nil
		default:
			return false, ErrApprovalExpired
		}
	case <-ctx.Done():
		if err := e.Store.DecideApproval(sctx, a.ID, "expired", "turn interrupted"); err == nil {
			a.Status = "expired"
			e.Bus.PublishAdmin(protocol.NotifyApprovalResolved, protocol.ApprovalEvent{Approval: a})
		}
		return false, ctx.Err()
	}
}

// RespondApproval records a decision. The first answer wins; later answers
// get a conflict error.
func (e *Engine) RespondApproval(ctx context.Context, id string, approve bool, clientID string) (protocol.Approval, error) {
	status := "denied"
	if approve {
		status = "approved"
	}
	if err := e.Store.DecideApproval(ctx, id, status, clientID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return protocol.Approval{}, protocol.Errorf(protocol.CodeConflict, "approval %s was already decided or does not exist", id)
		}
		return protocol.Approval{}, err
	}
	e.mu.Lock()
	ch := e.approvals[id]
	e.mu.Unlock()
	if ch != nil {
		ch <- approve
	}
	var decided protocol.Approval
	if list, err := e.Store.ListApprovals(ctx, ""); err == nil {
		for _, a := range list {
			if a.ID == id {
				decided = a
			}
		}
	}
	_ = e.Store.Audit(ctx, "approval.decide", map[string]any{"id": id, "status": status, "by": clientID})
	e.Bus.PublishAdmin(protocol.NotifyApprovalResolved, protocol.ApprovalEvent{Approval: decided})
	if decided.ID == "" {
		return decided, fmt.Errorf("approval %s not found after decision", id)
	}
	return decided, nil
}
