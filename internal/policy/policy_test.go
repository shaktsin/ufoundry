package policy

import (
	"testing"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/tools"
)

func TestGate(t *testing.T) {
	g := New(config.PolicyConfig{AutoApproveTools: []string{"file.*"}})
	cases := []struct {
		tool     string
		risk     tools.Risk
		listener bool
		want     Decision
	}{
		{"file.read", tools.RiskGreen, false, Allow},
		{"shell.run", tools.RiskRed, false, Ask},
		{"file.write", tools.RiskRed, false, Allow},
		{"file.write", tools.RiskRed, true, Ask},
		{"gmail.send", tools.RiskYellow, false, Allow},
	}
	for _, c := range cases {
		if got, _ := g.Check(c.tool, c.risk, c.listener); got != c.want {
			t.Errorf("%s/%s/%v = %s, want %s", c.tool, c.risk, c.listener, got, c.want)
		}
	}
	strict := New(config.PolicyConfig{ConfirmationStrictness: "strict"})
	if got, _ := strict.Check("gmail.send", tools.RiskYellow, false); got != Ask {
		t.Error("strict mode should ask for yellow")
	}
}
