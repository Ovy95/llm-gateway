package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

const UpstreamTimeout = 30 * time.Second

type Config struct {
	Keys        map[string]string
	UpstreamKey string
	UpstreamURL string
	Client      *http.Client
}
type API struct {
	validKeys   map[string]string
	upstreamKey string
	upstreamURL string
	client      *http.Client
}

func New(cfg Config) *API {
	return &API{
		validKeys:   cfg.Keys,
		upstreamKey: cfg.UpstreamKey,
		upstreamURL: cfg.UpstreamURL,
		client:      cfg.Client,
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.Health)
	mux.HandleFunc("POST /v1/chat/completions", a.ChatCompletions)
	return mux
}

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("ok"))
	if err != nil {
		return
	}
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

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.upstreamURL+"/v1/chat/completions", r.Body)
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	upstreamReq.Header.Set("Authorization", "Bearer "+a.upstreamKey)
	upstreamReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(upstreamReq)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
