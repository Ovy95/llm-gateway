package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github.com/ovy95/llm-gateway/internal/api"
)

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
		Keys:        validKeys,
		UpstreamKey: upstreamKey,
		UpstreamURL: upstreamURL,
		Client:      &http.Client{Timeout: api.UpstreamClientTimeout},
		Timeout:     api.DefaultUpstreamTimeout,
		Logger:      logger,
	}
	a := api.New(cfg)

	logger.Info("gateway listening", "port", port, "upstream", upstreamURL)
	log.Fatal(http.ListenAndServe(":"+port, a.Routes()))
}
