// Package policy decides whether a tool call runs, needs approval, or is denied.
package policy

import (
	"path"

	"github.com/shaktsin/umcode/internal/config"
	"github.com/shaktsin/umcode/internal/tools"
)

// Decision is the outcome of a policy check.
type Decision string

const (
	Allow Decision = "allow"
	Ask   Decision = "ask"
	Deny  Decision = "deny"
)

// Gate applies the policy config.
type Gate struct {
	cfg config.PolicyConfig
}

// New returns a Gate.
func New(cfg config.PolicyConfig) *Gate { return &Gate{cfg: cfg} }

// Check decides for a tool call with an assessed risk.
//
//   - green: allowed
//   - yellow: allowed, or approval in strict mode
//   - red: always needs approval
//
// Tools listed in policy.auto_approve_tools (glob patterns) skip approval,
// except red calls from listener channels (inbound email/Telegram), which
// always need an admin approval.
func (g *Gate) Check(tool string, risk tools.Risk, fromListener bool) (Decision, string) {
	if risk == tools.RiskRed && fromListener {
		return Ask, "red action triggered by an inbound message"
	}
	for _, pat := range g.cfg.AutoApproveTools {
		if ok, _ := path.Match(pat, tool); ok {
			return Allow, "auto-approved by policy.auto_approve_tools"
		}
	}
	switch risk {
	case tools.RiskGreen:
		return Allow, ""
	case tools.RiskYellow:
		if g.cfg.ConfirmationStrictness == "strict" {
			return Ask, "strict mode requires approval for yellow actions"
		}
		return Allow, ""
	default:
		return Ask, "red action requires approval"
	}
}
