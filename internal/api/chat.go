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
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	keyCfg, found := a.validKeys[key]

	if !ok || !found {
		writeError(w, http.StatusUnauthorized, "missing or invalid api key")
		return
	}

	if !a.limiterFor(key).Allow() {
		w.Header().Set("Retry-After", strconv.Itoa(60/a.rpmFor(key)))
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	if a.store.SpendUSD(key) >= keyCfg.BudgetUSD {
		writeError(w, http.StatusPaymentRequired, "spend cap exceeded") // 402
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()

	resp, err := a.forwardToUpstream(ctx, r)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusGatewayTimeout, "gateway timeout")
			return
		}
		writeError(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading upstream response")
		return
	}

	if resp.StatusCode == http.StatusOK {
		var parsed struct {
			Model string `json:"model"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			if cost, ok := costUSD(parsed.Model, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens); ok {
				a.store.Record(key, parsed.Usage.PromptTokens+parsed.Usage.CompletionTokens, cost)
			}
		}
	}

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)

}
