package models

import (
	"context"
	"math"
	"testing"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
)

func TestClassify(t *testing.T) {
	cases := map[string]protocol.Complexity{
		"hi":                          protocol.ComplexityQuick,
		"what is the capital of peru": protocol.ComplexityQuick,
		"Research the top three vector databases and compare their pricing":                       protocol.ComplexityDeep,
		"Rename the files in ~/Downloads that start with IMG to photo-N and tell me what changed": protocol.ComplexityStandard,
	}
	for in, want := range cases {
		if got := Classify(in, 0); got != want {
			t.Errorf("Classify(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestPresetsOverride(t *testing.T) {
	p := Presets(config.ModelsConfig{Complexity: map[string]config.ComplexityPreset{"deep": {MaxToolSteps: 60}}})
	if p[protocol.ComplexityDeep].MaxToolSteps != 60 || p[protocol.ComplexityDeep].Reasoning != llm.ReasoningHigh {
		t.Fatalf("override not applied: %+v", p[protocol.ComplexityDeep])
	}
}

func TestCostAndCatalog(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c, err := NewCatalog(st)
	if err != nil {
		t.Fatal(err)
	}
	m := c.Lookup(ctx, "claude", "claude-sonnet-4-6")
	// 1M input of which 400k cached, 100k output: 0.6*3 + 0.4*0.3 + 0.1*15 = 3.42
	got := Cost(m, llm.Usage{InputTokens: 1_000_000, CachedInputTokens: 400_000, OutputTokens: 100_000})
	if math.Abs(got-3.42) > 1e-9 {
		t.Fatalf("cost = %v", got)
	}
	if err := st.SetModelPrice(ctx, protocol.ModelSetPriceParams{Provider: "claude", Model: "claude-sonnet-4-6", InputPerMTok: 1, OutputPerMTok: 1}); err != nil {
		t.Fatal(err)
	}
	if m := c.Lookup(ctx, "claude", "claude-sonnet-4-6"); m.In != 1 {
		t.Fatalf("override ignored: %+v", m)
	}
	if err := st.ReplaceModelCache(ctx, "openai", []string{"gpt-9-test", "text-embedding-3-large"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetModelHidden(ctx, "gemini", "gemini-2.5-pro", true); err != nil {
		t.Fatal(err)
	}
	list, err := c.List(ctx, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var sawAPI, sawEmbed, sawHidden bool
	for _, m := range list {
		sawAPI = sawAPI || m.ID == "gpt-9-test"
		sawEmbed = sawEmbed || m.ID == "text-embedding-3-large"
		sawHidden = sawHidden || m.ID == "gemini-2.5-pro"
	}
	if !sawAPI || sawEmbed || sawHidden {
		t.Fatalf("api=%v embed=%v hidden=%v", sawAPI, sawEmbed, sawHidden)
	}
	if unk := c.Lookup(ctx, "openai", "gpt-4o-mini"); Cost(unk, llm.Usage{InputTokens: 1000}) != 0 {
		t.Fatal("unknown price must cost 0")
	}
}
