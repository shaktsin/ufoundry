package credentials

import (
	"context"
	"testing"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/secrets"
	"github.com/shaktsin/ufoundry/internal/store"
)

func TestImportLegacyKeys(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sec := secrets.NewMemoryStore()
	s := New(st, sec, llm.NewRegistry())
	cfg := config.Default(t.TempDir())
	cfg.LLM.Provider = "openai"
	legacy := map[string]string{
		"UMABOT_LLM_API_KEY":           "sk-main-openai", // pre-rename main key → openai
		"UFOUNDRY_LLM_GEMINI_API_KEY":  "gm-key-1111",    // provider-specific key
		"UMABOT_LLM_ANTHROPIC_API_KEY": "sk-ant-2222",    // alias
	}
	added, err := s.ImportConfigKeys(ctx, cfg, func(n string) string { return legacy[n] })
	if err != nil || len(added) != 3 {
		t.Fatalf("added=%+v err=%v", added, err)
	}
	for _, prov := range []string{"openai", "gemini", "claude"} {
		r, err := s.Resolve(ctx, prov, "")
		if err != nil {
			t.Fatalf("%s: %v", prov, err)
		}
		if r.Material.APIKey == "" || r.Record.Label != "Imported" {
			t.Fatalf("%s: %+v", prov, r)
		}
	}
	// Second run imports nothing.
	added, _ = s.ImportConfigKeys(ctx, cfg, func(n string) string { return legacy[n] })
	if len(added) != 0 {
		t.Fatalf("re-imported: %+v", added)
	}
}
