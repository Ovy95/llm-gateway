package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const DefaultUpstreamTimeout = 30 * time.Second
const UpstreamClientTimeout = DefaultUpstreamTimeout + 5*time.Second

type KeyConfig struct {
	Tenant    string
	BudgetUSD float64
	RPM       int // requests per minute; burst = RPM
}
type Config struct {
	Keys        map[string]KeyConfig
	UpstreamKey string
	UpstreamURL string
	Client      *http.Client
	Timeout     time.Duration
	Logger      *slog.Logger
	Store       UsageStore
}
type API struct {
	validKeys   map[string]KeyConfig
	upstreamKey string
	upstreamURL string
	client      *http.Client
	timeout     time.Duration
	limiters    map[string]*rate.Limiter
	mu          sync.Mutex
	store       UsageStore
	logger      *slog.Logger
}

func New(cfg Config) *API {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultUpstreamTimeout
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	store := cfg.Store
	if store == nil {
		store = newMemoryStore()
	}

	return &API{
		validKeys:   cfg.Keys,
		upstreamKey: cfg.UpstreamKey,
		upstreamURL: cfg.UpstreamURL,
		client:      cfg.Client,
		timeout:     timeout,
		limiters:    make(map[string]*rate.Limiter),
		store:       store,
		logger:      logger,
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.Health)
	mux.HandleFunc("POST /v1/chat/completions", a.ChatCompletions)
	mux.HandleFunc("GET /stats", a.Stats)
	return mux
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
