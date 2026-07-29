package api

import (
	"log/slog"
	"time"
)

type auditEntry struct {
	tenant           string
	model            string
	promptTokens     int
	completionTokens int
	costUSD          float64
	status           int
	latency          time.Duration
	decision         string
}

func (a *API) audit(e auditEntry) {
	a.logger.Info("request",
		slog.String("tenant", e.tenant),
		slog.String("model", e.model),
		slog.Int("prompt_tokens", e.promptTokens),
		slog.Int("completion_tokens", e.completionTokens),
		slog.Float64("cost_usd", e.costUSD),
		slog.Int("status", e.status),
		slog.Int64("latency_ms", e.latency.Milliseconds()),
		slog.String("decision", e.decision),
	)
}
