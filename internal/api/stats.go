package api

import (
	"encoding/json"
	"net/http"
)

type keyStats struct {
	Requests           int     `json:"requests"`
	Tokens             int     `json:"tokens"`
	SpendUSD           float64 `json:"spend_usd"`
	BudgetRemainingUSD float64 `json:"budget_remaining_usd"`
}

func (a *API) Stats(w http.ResponseWriter, r *http.Request) {
	usage := a.store.CurrentUsage()

	out := make(map[string]keyStats, len(a.validKeys))
	for key, cfg := range a.validKeys {
		u := usage[key]
		out[cfg.Tenant] = keyStats{
			Requests:           u.Requests,
			Tokens:             u.Tokens,
			SpendUSD:           u.SpendUSD,
			BudgetRemainingUSD: cfg.BudgetUSD - u.SpendUSD,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
