package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/shaktsin/ufoundry/internal/protocol"
)

type handler func(ctx context.Context, c *conn, params json.RawMessage) (any, error)

// bind decodes params into P and calls fn.
func bind[P any](fn func(ctx context.Context, c *conn, p P) (any, error)) handler {
	return func(ctx context.Context, c *conn, raw json.RawMessage) (any, error) {
		var p P
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, protocol.Errorf(protocol.CodeInvalidParams, "invalid params: %v", err)
			}
		}
		return fn(ctx, c, p)
	}
}

type empty struct{}

type okResult struct {
	OK bool `json:"ok"`
}

func (s *Server) routes() map[string]handler {
	e := s.eng
	return map[string]handler{
		protocol.MethodInitialize: bind(func(ctx context.Context, c *conn, p protocol.InitializeParams) (any, error) {
			if major(p.ProtocolVersion) != major(protocol.Version) {
				return nil, protocol.Errorf(protocol.CodeVersionMismatch,
					"client speaks protocol %s, engine speaks %s", p.ProtocolVersion, protocol.Version)
			}
			c.admin.Store(p.Admin)
			c.initialized.Store(true)
			e.Bus.Add(c)
			return protocol.InitializeResult{EngineVersion: engineVersion(), ProtocolVersion: protocol.Version, ClientID: c.id}, nil
		}),
		protocol.MethodEngineStatus: bind(func(ctx context.Context, c *conn, _ empty) (any, error) {
			return e.Status(ctx), nil
		}),
		protocol.MethodEventsSubscribe: bind(func(ctx context.Context, c *conn, p protocol.SubscribeParams) (any, error) {
			c.subscribe(p)
			return okResult{true}, nil
		}),

		// threads
		protocol.MethodThreadStart: bind(func(ctx context.Context, c *conn, p protocol.ThreadStartParams) (any, error) {
			t, err := e.StartThread(ctx, p)
			if err == nil {
				c.follow(t.ID)
			}
			return t, err
		}),
		protocol.MethodThreadList: bind(func(ctx context.Context, c *conn, p protocol.ThreadListParams) (any, error) {
			ts, next, err := e.Store.ListThreads(ctx, p)
			if ts == nil {
				ts = []protocol.Thread{}
			}
			return protocol.ThreadListResult{Threads: ts, NextCursor: next}, err
		}),
		protocol.MethodThreadRead: bind(func(ctx context.Context, c *conn, p protocol.ThreadIDParams) (any, error) {
			r, err := e.ReadThread(ctx, p.ThreadID)
			if err == nil {
				c.follow(p.ThreadID)
			}
			return r, err
		}),
		protocol.MethodThreadRename: bind(func(ctx context.Context, c *conn, p protocol.ThreadRenameParams) (any, error) {
			title := strings.TrimSpace(p.Title)
			if title == "" {
				return nil, protocol.Errorf(protocol.CodeInvalidParams, "title is empty")
			}
			return e.UpdateThread(ctx, p.ThreadID, map[string]any{"title": title})
		}),
		protocol.MethodThreadPin: bind(func(ctx context.Context, c *conn, p protocol.ThreadFlagParams) (any, error) {
			return e.UpdateThread(ctx, p.ThreadID, map[string]any{"pinned": p.Value})
		}),
		protocol.MethodThreadArchive: bind(func(ctx context.Context, c *conn, p protocol.ThreadFlagParams) (any, error) {
			return e.UpdateThread(ctx, p.ThreadID, map[string]any{"archived": p.Value})
		}),
		protocol.MethodThreadDelete: bind(func(ctx context.Context, c *conn, p protocol.ThreadIDParams) (any, error) {
			return okResult{true}, e.DeleteThread(ctx, p.ThreadID)
		}),
		protocol.MethodThreadFork: bind(func(ctx context.Context, c *conn, p protocol.ThreadForkParams) (any, error) {
			t, err := e.ForkThread(ctx, p)
			if err == nil {
				c.follow(t.ID)
			}
			return t, err
		}),
		protocol.MethodThreadSearch: bind(func(ctx context.Context, c *conn, p protocol.ThreadSearchParams) (any, error) {
			hits, err := e.Store.SearchItems(ctx, p.Query, p.Limit)
			if hits == nil {
				hits = []protocol.SearchHit{}
			}
			return protocol.ThreadSearchResult{Hits: hits}, err
		}),
		protocol.MethodThreadExport: bind(func(ctx context.Context, c *conn, p protocol.ThreadIDParams) (any, error) {
			md, err := e.ExportThread(ctx, p.ThreadID)
			return protocol.ThreadExportResult{Markdown: md}, err
		}),
		protocol.MethodThreadSetSettings: bind(func(ctx context.Context, c *conn, p protocol.ThreadSetSettingsParams) (any, error) {
			return e.SetThreadSettings(ctx, p)
		}),

		// turns
		protocol.MethodTurnStart: bind(func(ctx context.Context, c *conn, p protocol.TurnStartParams) (any, error) {
			c.follow(p.ThreadID)
			t, err := e.StartTurn(ctx, p)
			return protocol.TurnStartResult{Turn: t}, err
		}),
		protocol.MethodTurnInterrupt: bind(func(ctx context.Context, c *conn, p protocol.TurnInterruptParams) (any, error) {
			return okResult{true}, e.InterruptTurn(p.TurnID)
		}),

		// approvals
		protocol.MethodApprovalList: bind(func(ctx context.Context, c *conn, _ empty) (any, error) {
			list, err := e.Store.ListApprovals(ctx, "pending")
			if list == nil {
				list = []protocol.Approval{}
			}
			return protocol.ApprovalListResult{Approvals: list}, err
		}),
		protocol.MethodApprovalRespond: bind(func(ctx context.Context, c *conn, p protocol.ApprovalRespondParams) (any, error) {
			if !c.IsAdmin() {
				return nil, protocol.Errorf(protocol.CodeInvalidRequest, "only admin clients can answer approvals")
			}
			return e.RespondApproval(ctx, p.ApprovalID, p.Approve, c.id)
		}),

		// providers and models
		protocol.MethodProviderList: bind(func(ctx context.Context, c *conn, _ empty) (any, error) {
			ps, err := e.Providers(ctx)
			return protocol.ProviderListResult{Providers: ps}, err
		}),
		protocol.MethodModelList: bind(func(ctx context.Context, c *conn, p protocol.ModelListParams) (any, error) {
			ms, err := e.Catalog.List(ctx, p.Provider, p.IncludeHidden)
			if ms == nil {
				ms = []protocol.Model{}
			}
			return protocol.ModelListResult{Models: ms}, err
		}),
		protocol.MethodModelSetHidden: bind(func(ctx context.Context, c *conn, p protocol.ModelSetHiddenParams) (any, error) {
			return okResult{true}, e.Store.SetModelHidden(ctx, p.Provider, p.Model, p.Hidden)
		}),
		protocol.MethodModelSetPrice: bind(func(ctx context.Context, c *conn, p protocol.ModelSetPriceParams) (any, error) {
			if p.InputPerMTok < 0 || p.OutputPerMTok < 0 || p.CachedInputPerMTok < 0 {
				return nil, protocol.Errorf(protocol.CodeInvalidParams, "prices must be >= 0")
			}
			return okResult{true}, e.Store.SetModelPrice(ctx, p)
		}),
		protocol.MethodModelRefresh: bind(func(ctx context.Context, c *conn, p protocol.ModelRefreshParams) (any, error) {
			return e.RefreshModels(ctx, p.CredentialID)
		}),

		// API keys
		protocol.MethodCredentialList: bind(func(ctx context.Context, c *conn, _ empty) (any, error) {
			cs, err := e.Creds.List(ctx)
			if cs == nil {
				cs = []protocol.Credential{}
			}
			return protocol.CredentialListResult{Credentials: cs}, err
		}),
		protocol.MethodCredentialAdd: bind(func(ctx context.Context, c *conn, p protocol.CredentialAddParams) (any, error) {
			cred, err := e.Creds.Add(ctx, p)
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
			}
			return cred, nil
		}),
		protocol.MethodCredentialTest: bind(func(ctx context.Context, c *conn, p protocol.CredentialIDParams) (any, error) {
			return e.Creds.Test(ctx, p.CredentialID)
		}),
		protocol.MethodCredentialUpdate: bind(func(ctx context.Context, c *conn, p protocol.CredentialUpdateParams) (any, error) {
			return e.Creds.Update(ctx, p)
		}),
		protocol.MethodCredentialRotate: bind(func(ctx context.Context, c *conn, p protocol.CredentialRotateParams) (any, error) {
			return e.Creds.Rotate(ctx, p.CredentialID, p.Secret)
		}),
		protocol.MethodCredentialDelete: bind(func(ctx context.Context, c *conn, p protocol.CredentialIDParams) (any, error) {
			return okResult{true}, e.Creds.Delete(ctx, p.CredentialID)
		}),

		// usage
		protocol.MethodUsageSummary: bind(func(ctx context.Context, c *conn, p protocol.UsageSummaryParams) (any, error) {
			r, err := e.UsageSummary(ctx, p)
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
			}
			if r.Rows == nil {
				r.Rows = []protocol.UsageRow{}
			}
			return r, nil
		}),
		protocol.MethodUsageSetBudget: bind(func(ctx context.Context, c *conn, p protocol.UsageSetBudgetParams) (any, error) {
			return okResult{true}, e.Creds.SetBudget(ctx, p)
		}),

		// complexity
		protocol.MethodComplexityGetDefaults: bind(func(ctx context.Context, c *conn, _ empty) (any, error) {
			return e.ComplexityDefaults(), nil
		}),
		protocol.MethodComplexitySetDefaults: bind(func(ctx context.Context, c *conn, p protocol.ComplexityDefaults) (any, error) {
			d, err := e.SetComplexityDefaults(ctx, p)
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
			}
			return d, nil
		}),
	}
}

func major(v string) string {
	if i := strings.IndexByte(v, '.'); i >= 0 {
		return v[:i]
	}
	return v
}
