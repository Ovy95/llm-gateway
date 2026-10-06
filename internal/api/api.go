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

// Default ceilings on request/response bodies so a single call can't exhaust
// gateway memory — prompts/completions are text, so these are generous.
const DefaultMaxRequestBytes = 1 << 20   // 1 MiB
const DefaultMaxResponseBytes = 10 << 20 // 10 MiB

type KeyConfig struct {
	Tenant    string
	BudgetUSD float64
	RPM       int // requests per minute; burst = RPM
}
type Config struct {
	Keys             map[string]KeyConfig
	UpstreamKey      string
	UpstreamURL      string
	Client           *http.Client
	Timeout          time.Duration
	Logger           *slog.Logger
	Store            UsageStore
	MaxRequestBytes  int64
	MaxResponseBytes int64
}
type API struct {
	validKeys        map[string]KeyConfig
	upstreamKey      string
	upstreamURL      string
	client           *http.Client
	timeout          time.Duration
	limiters         map[string]*rate.Limiter
	mu               sync.Mutex
	budgetLocks      map[string]keyMutex
	store            UsageStore
	logger           *slog.Logger
	maxRequestBytes  int64
	maxResponseBytes int64
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

	maxRequestBytes := cfg.MaxRequestBytes
	if maxRequestBytes <= 0 {
		maxRequestBytes = DefaultMaxRequestBytes
	}
	maxResponseBytes := cfg.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}

	return &API{
		validKeys:        cfg.Keys,
		upstreamKey:      cfg.UpstreamKey,
		upstreamURL:      cfg.UpstreamURL,
		client:           cfg.Client,
		timeout:          timeout,
		limiters:         make(map[string]*rate.Limiter),
		budgetLocks:      make(map[string]keyMutex),
		store:            store,
		logger:           logger,
		maxRequestBytes:  maxRequestBytes,
		maxResponseBytes: maxResponseBytes,
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
