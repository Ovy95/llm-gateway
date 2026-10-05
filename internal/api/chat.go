package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *API) forwardToUpstream(ctx context.Context, r *http.Request) (*http.Response, error) {
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.upstreamURL+"/v1/chat/completions", r.Body)
	if err != nil {
		return nil, fmt.Errorf("building upstream request: %w", err)
	}

	upstreamReq.Header.Set("Authorization", "Bearer "+a.upstreamKey)
	upstreamReq.Header.Set("Content-Type", "application/json")

	return a.client.Do(upstreamReq)
}

func (a *API) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	entry := auditEntry{decision: "unauthorized", status: http.StatusUnauthorized}
	defer func() {
		entry.latency = time.Since(start)
		a.audit(entry)
	}()

	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	keyCfg, found := a.validKeys[key]

	if !ok || !found {
		writeError(w, http.StatusUnauthorized, "missing or invalid api key")
		return
	}
	entry.tenant = keyCfg.Tenant

	if !a.limiterFor(key).Allow() {
		entry.decision = "rate_limited"
		entry.status = http.StatusTooManyRequests
		w.Header().Set("Retry-After", strconv.Itoa(60/a.rpmFor(key)))
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	// Held from the budget check through Record so two concurrent requests
	// on the same key can't both slip under the cap before either records.
	budgetMu := a.budgetMuFor(key)
	budgetMu.Lock()
	defer budgetMu.Unlock()

	if a.store.SpendUSD(key) >= keyCfg.BudgetUSD {
		entry.decision = "budget_exceeded"
		entry.status = http.StatusPaymentRequired
		writeError(w, http.StatusPaymentRequired, "spend cap exceeded") // 402
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()

	resp, err := a.forwardToUpstream(ctx, r)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			entry.decision = "upstream_timeout"
			entry.status = http.StatusGatewayTimeout
			writeError(w, http.StatusGatewayTimeout, "gateway timeout")
			return
		}
		entry.decision = "upstream_unreachable"
		entry.status = http.StatusBadGateway
		writeError(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	defer resp.Body.Close()

	entry.status = resp.StatusCode

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		entry.decision = "upstream_unreachable"
		entry.status = http.StatusBadGateway
		writeError(w, http.StatusBadGateway, "reading upstream response")
		return
	}

	if resp.StatusCode == http.StatusOK {
		entry.decision = "allowed_unbilled" // overwritten below once cost is known
		var parsed struct {
			Model string `json:"model"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			entry.model = parsed.Model
			entry.promptTokens = parsed.Usage.PromptTokens
			entry.completionTokens = parsed.Usage.CompletionTokens
			if cost, ok := costUSD(parsed.Model, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens); ok {
				a.store.Record(key, parsed.Usage.PromptTokens+parsed.Usage.CompletionTokens, cost)
				entry.costUSD = cost
				entry.decision = "allowed"
			}
		}
	} else {
		entry.decision = "upstream_error"
	}

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
