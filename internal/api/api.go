package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const DefaultUpstreamTimeout = 30 * time.Second
const UpstreamClientTimeout = DefaultUpstreamTimeout + 5*time.Second

type Config struct {
	Keys        map[string]string
	UpstreamKey string
	UpstreamURL string
	Client      *http.Client
	Timeout     time.Duration
}
type API struct {
	validKeys   map[string]string
	upstreamKey string
	upstreamURL string
	client      *http.Client
	timeout     time.Duration
	limiters    map[string]*rate.Limiter
	mu          sync.Mutex
}

func New(cfg Config) *API {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultUpstreamTimeout
	}

	return &API{
		validKeys:   cfg.Keys,
		upstreamKey: cfg.UpstreamKey,
		upstreamURL: cfg.UpstreamURL,
		client:      cfg.Client,
		timeout:     timeout,
		limiters:    make(map[string]*rate.Limiter),
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.Health)
	mux.HandleFunc("POST /v1/chat/completions", a.ChatCompletions)
	return mux
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
