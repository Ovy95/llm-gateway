package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github.com/ovy95/llm-gateway/internal/api"
)

func main() {
	_ = godotenv.Load()
	validKeys := map[string]string{
		"sk-demo-alice": "alice",
		"sk-demo-bob":   "bob",
	}

	upstreamURL := os.Getenv("UPSTREAM_URL")
	if upstreamURL == "" {
		upstreamURL = "https://api.openai.com"
	}

	upstreamKey := os.Getenv("OPENAI_API_KEY")
	if upstreamKey == "" {
		log.Fatal("OPENAI_API_KEY must be set")
	}

	cfg := api.Config{
		Keys:        validKeys,
		UpstreamKey: upstreamKey,
		UpstreamURL: upstreamURL,
		Client:      &http.Client{Timeout: api.UpstreamTimeout},
	}
	a := api.New(cfg)

	log.Fatal(http.ListenAndServe(":8080", a.Routes()))
}
