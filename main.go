package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/ovy95/llm-gateway/internal/api"
)

// getEnvBytes reads an optional byte-count env var (e.g. "2097152" for 2
// MiB). An unset var returns 0, which tells api.New to fall back to its
// built-in default; a set-but-invalid value fails fast rather than
// silently running on the default the operator tried to override.
func getEnvBytes(name string) int64 {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("%s must be a byte count (integer): %v", name, err)
	}
	return n
}

func main() {
	_ = godotenv.Load()

	validKeys := map[string]api.KeyConfig{
		"sk-demo-alice": {Tenant: "alice", BudgetUSD: 0.10, RPM: 5},
		"sk-demo-bob":   {Tenant: "bob", BudgetUSD: 0.01, RPM: 2},
	}

	upstreamURL := os.Getenv("UPSTREAM_URL")
	if upstreamURL == "" {
		upstreamURL = "https://api.openai.com"
	}

	upstreamKey := os.Getenv("OPENAI_API_KEY")
	if upstreamKey == "" {
		log.Fatal("OPENAI_API_KEY must be set")
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg := api.Config{
		Keys:             validKeys,
		UpstreamKey:      upstreamKey,
		UpstreamURL:      upstreamURL,
		Client:           &http.Client{Timeout: api.UpstreamClientTimeout},
		Timeout:          api.DefaultUpstreamTimeout,
		Logger:           logger,
		MaxRequestBytes:  getEnvBytes("MAX_REQUEST_BYTES"),
		MaxResponseBytes: getEnvBytes("MAX_RESPONSE_BYTES"),
	}
	a := api.New(cfg)

	logger.Info("gateway listening", "port", port, "upstream", upstreamURL)
	log.Fatal(http.ListenAndServe(":"+port, a.Routes()))
}
