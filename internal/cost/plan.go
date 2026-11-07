package cost

import (
	"strings"

	"lo0.ai/lo0/internal/claudecode/setup"
)

// Plan is a subscription and what it costs.
type Plan struct {
	Name       string  `json:"name"`
	MonthlyUSD float64 `json:"monthlyUsd,omitzero"`
	// FromUSD is the cheapest tier's price when the plan has tiers lo0
	// cannot tell apart.
	FromUSD float64 `json:"fromUsd,omitzero"`
	// Known is false when lo0 could not tell the plan or does not know its
	// price, for example team and enterprise seats.
	Known bool `json:"known"`
	// PerToken is true when usage is billed per token rather than by plan.
	PerToken bool `json:"perToken,omitzero"`
}

// PlanFor names the plan and its list price from claude.com/pricing. Since
// 30 October 2026 the $100 plan is called Max and the $200 one, formerly
// Max 20x, Ultra; lo0 tells them apart by the account's 20x tier.
func PlanFor(a setup.Account) Plan {
	switch a.Plan {
	case setup.PlanPro:
		return Plan{Name: "Pro", MonthlyUSD: 20, Known: true}
	case setup.PlanMax:
		if a.Tier == "20x" {
			return Plan{Name: "Ultra", MonthlyUSD: 200, Known: true}
		}
		return Plan{Name: "Max", MonthlyUSD: 100, Known: true}
	case setup.PlanAPI:
		return Plan{Name: "API key", Known: true, PerToken: true}
	case setup.PlanTeam:
		return Plan{Name: "Team"}
	case setup.PlanEnterprise:
		return Plan{Name: "Enterprise"}
	default:
		return Plan{Name: "unknown"}
	}
}

// ChatGPTPlanFor names the ChatGPT plan Codex reports as its plan type, with
// the monthly list price chatgpt.com/pricing showed on 6 November 2026. Pro
// has three tiers from $100 that Codex does not tell apart, and Business is
// priced by seat type and billing period.
func ChatGPTPlanFor(planType string) Plan {
	switch strings.ToLower(planType) {
	case "free":
		return Plan{Name: "ChatGPT Free", Known: true}
	case "go":
		return Plan{Name: "ChatGPT Go", MonthlyUSD: 8, Known: true}
	case "plus":
		return Plan{Name: "ChatGPT Plus", MonthlyUSD: 20, Known: true}
	case "pro":
		return Plan{Name: "ChatGPT Pro", FromUSD: 100}
	case "team", "business":
		return Plan{Name: "ChatGPT Business"}
	case "enterprise":
		return Plan{Name: "ChatGPT Enterprise"}
	case "edu":
		return Plan{Name: "ChatGPT Edu"}
	default:
		return Plan{Name: "unknown"}
	}
}
