package api

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
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
	limiters    map[string]*rate.Limiter
	mu          sync.Mutex
}

func New(cfg Config) *API {
	return &API{
		validKeys:   cfg.Keys,
		upstreamKey: cfg.UpstreamKey,
		upstreamURL: cfg.UpstreamURL,
		client:      cfg.Client,
		limiters:    make(map[string]*rate.Limiter),
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.Health)
	mux.HandleFunc("POST /v1/chat/completions", a.ChatCompletions)
	return mux
}
