package router

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/credentials"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/models"
	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/secrets"
	"github.com/shaktsin/ufoundry/internal/store"
)

func newRouter(t *testing.T) (*Router, *credentials.Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	llms := llm.NewRegistry()
	creds := credentials.New(st, secrets.NewMemoryStore(), llms)
	cat, err := models.NewCatalog(st)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(t.TempDir())
	cfg.LLM.Provider = "claude"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(st, creds, cat, llms, cfg, log), creds, st
}

func addKey(t *testing.T, creds *credentials.Service, provider, label, secret string) protocol.Credential {
	t.Helper()
	c, err := creds.Add(context.Background(), protocol.CredentialAddParams{Provider: provider, Label: label, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPlanWithoutKeysExplainsItself(t *testing.T) {
	r, _, _ := newRouter(t)
	plan, err := r.Plan(context.Background(), Request{Level: protocol.ComplexityStandard})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Routes) != 0 || plan.Reason == "" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestPlanLinesUpFallbacks(t *testing.T) {
	ctx := context.Background()
	r, creds, _ := newRouter(t)
	k1 := addKey(t, creds, "claude", "primary", "sk-one")
	k2 := addKey(t, creds, "claude", "backup", "sk-two")

	plan, err := r.Plan(ctx, Request{Level: protocol.ComplexityStandard, Need: Capabilities{Tools: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Routes) < 2 {
		t.Fatalf("expected fallbacks, got %+v", plan.Routes)
	}
	first, _ := plan.Next()
	if first.Provider != "claude" {
		t.Fatalf("first route = %+v", first)
	}
	// Every route is usable, and the same model/key pair never repeats.
	seen := map[string]bool{}
	for _, rt := range plan.Routes {
		key := rt.Model + "/" + rt.Cred.Record.ID
		if seen[key] {
			t.Fatalf("route repeated: %s", key)
		}
		seen[key] = true
	}
	var sawBoth bool
	for _, rt := range plan.Routes {
		if rt.Model == first.Model && rt.Cred.Record.ID != first.Cred.Record.ID {
			sawBoth = true
		}
	}
	if !sawBoth {
		t.Fatalf("the second key is never tried: %+v", plan.Routes)
	}
	_ = k1
	_ = k2
}

func TestPinnedKeyIsExclusive(t *testing.T) {
	ctx := context.Background()
	r, creds, _ := newRouter(t)
	addKey(t, creds, "claude", "primary", "sk-one")
	k2 := addKey(t, creds, "claude", "backup", "sk-two")

	plan, err := r.Plan(ctx, Request{Level: protocol.ComplexityStandard,
		Pinned: protocol.ModelSelection{CredentialID: k2.ID}, Need: Capabilities{Tools: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Routes) == 0 {
		t.Fatal("no routes")
	}
	for _, rt := range plan.Routes {
		if rt.Cred.Record.ID != k2.ID {
			t.Fatalf("pinned key ignored: %+v", rt)
		}
	}
}

func TestCooldownRemovesARoute(t *testing.T) {
	ctx := context.Background()
	r, creds, st := newRouter(t)
	k := addKey(t, creds, "claude", "only", "sk-one")

	plan, _ := r.Plan(ctx, Request{Level: protocol.ComplexityStandard, Need: Capabilities{Tools: true}})
	if len(plan.Routes) == 0 {
		t.Fatal("no routes")
	}
	hot := plan.Routes[0]
	if err := st.RecordFailure(ctx, hot.Provider, hot.Model, k.ID, "rate_limited", "429", time.Hour); err != nil {
		t.Fatal(err)
	}
	again, _ := r.Plan(ctx, Request{Level: protocol.ComplexityStandard, Need: Capabilities{Tools: true}})
	for _, rt := range again.Routes {
		if rt.Same(hot) {
			t.Fatalf("cooling-down route still offered: %+v", rt)
		}
	}
	var blocked bool
	for _, b := range again.Blocked {
		if b.Model == hot.Model && b.CooldownEnd != nil {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("cooldown not explained: %+v", again.Blocked)
	}

	// Clearing it puts the route back.
	if err := st.ClearCooldown(ctx, hot.Provider, hot.Model, k.ID); err != nil {
		t.Fatal(err)
	}
	back, _ := r.Plan(ctx, Request{Level: protocol.ComplexityStandard, Need: Capabilities{Tools: true}})
	var found bool
	for _, rt := range back.Routes {
		found = found || rt.Same(hot)
	}
	if !found {
		t.Fatal("route did not come back after the cooldown was cleared")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		status  string
		cooling bool
		retry   bool
	}{
		{"rate limit honours retry-after", &llm.Error{Status: http.StatusTooManyRequests, RetryAfter: 5 * time.Second}, "rate_limited", true, true},
		{"overloaded", &llm.Error{Status: 529}, "rate_limited", true, true},
		{"revoked key", &llm.Error{Status: http.StatusUnauthorized}, "unauthorized", true, true},
		{"outage", &llm.Error{Status: http.StatusBadGateway}, "server_error", true, true},
		{"unknown model", &llm.Error{Status: http.StatusNotFound}, "unknown_model", true, true},
		{"context too long", &llm.Error{Status: http.StatusBadRequest, Body: "maximum context length exceeded"}, "context_too_long", false, true},
		{"bad request", &llm.Error{Status: http.StatusBadRequest, Body: "nope"}, "invalid_request", false, false},
		{"timeout", context.DeadlineExceeded, "timeout", true, true},
		{"plain error", errors.New("boom"), "error", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, cooldown := Classify(c.err)
			if status != c.status {
				t.Fatalf("status = %q, want %q", status, c.status)
			}
			if (cooldown > 0) != c.cooling {
				t.Fatalf("cooldown = %v", cooldown)
			}
			if Retryable(c.err) != c.retry {
				t.Fatalf("retryable = %v", Retryable(c.err))
			}
		})
	}
	if _, cd := Classify(&llm.Error{Status: 429, RetryAfter: 5 * time.Second}); cd != 5*time.Second {
		t.Fatalf("retry-after ignored: %v", cd)
	}
	if _, cd := Classify(&llm.Error{Status: 429, RetryAfter: 10 * time.Hour}); cd > 30*time.Minute {
		t.Fatalf("retry-after not capped: %v", cd)
	}
}
