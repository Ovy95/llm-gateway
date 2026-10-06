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
	"sync"
	"time"
)

// statusClientClosedRequest follows the widely used (if non-standard, e.g.
// nginx) convention for a client that disconnected before the server
// finished responding — distinct from a gateway-side timeout (504) or a
// bad upstream response (502).
const statusClientClosedRequest = 499

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

	r.Body = http.MaxBytesReader(w, r.Body, a.maxRequestBytes)

	if !a.limiterFor(key).Allow() {
		entry.decision = "rate_limited"
		entry.status = http.StatusTooManyRequests
		w.Header().Set("Retry-After", strconv.Itoa(60/a.rpmFor(key)))
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	// Held from the budget check through Record so two concurrent requests
	// on the same key can't both slip under the cap before either records.
	// Lock respects the request's context, so a client that has already
	// disconnected doesn't sit queued behind others on a busy key holding
	// its goroutine for nothing; it's released as soon as the budget
	// decision is final (below) rather than for the rest of the handler,
	// so it doesn't also serialize writing the response back to slow
	// clients.
	budgetMu := a.budgetMuFor(key)
	if err := budgetMu.Lock(r.Context()); err != nil {
		entry.decision = "canceled"
		entry.status = statusClientClosedRequest
		writeError(w, statusClientClosedRequest, "client disconnected")
		return
	}
	unlockBudget := sync.OnceFunc(budgetMu.Unlock)
	defer unlockBudget()

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
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			entry.decision = "request_too_large"
			entry.status = http.StatusRequestEntityTooLarge
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		case errors.Is(err, context.DeadlineExceeded):
			entry.decision = "upstream_timeout"
			entry.status = http.StatusGatewayTimeout
			writeError(w, http.StatusGatewayTimeout, "gateway timeout")
		default:
			entry.decision = "upstream_unreachable"
			entry.status = http.StatusBadGateway
			writeError(w, http.StatusBadGateway, "upstream request failed")
		}
		return
	}
	defer resp.Body.Close()

	entry.status = resp.StatusCode

	// +1 so a body exactly at the limit still reads fully, while anything
	// past it trips the length check below instead of being read in full.
	limited := io.LimitReader(resp.Body, a.maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		entry.decision = "upstream_unreachable"
		entry.status = http.StatusBadGateway
		writeError(w, http.StatusBadGateway, "reading upstream response")
		return
	}
	if int64(len(body)) > a.maxResponseBytes {
		entry.decision = "upstream_response_too_large"
		entry.status = http.StatusBadGateway
		writeError(w, http.StatusBadGateway, "upstream response too large")
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

	// The budget decision (and any Record) is final at this point; release
	// the lock before writing the response so a slow client can't also
	// hold up the next request on this key.
	unlockBudget()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
