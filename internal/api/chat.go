package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	_, found := a.validKeys[key]

	if !ok || !found {
		http.Error(w, "missing or invalid api key", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), UpstreamTimeout)
	defer cancel()

	resp, err := a.forwardToUpstream(ctx, r)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
