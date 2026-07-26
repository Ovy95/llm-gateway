package api

import (
	"context"
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

	_, found := a.validKeys[key]

	if !ok || !found {
		writeError(w, http.StatusUnauthorized, "missing or invalid api key")
		return
	}

	if !a.limiterFor(key).Allow() {
		w.Header().Set("Retry-After", strconv.Itoa(60/a.rpmFor(key)))
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
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

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
