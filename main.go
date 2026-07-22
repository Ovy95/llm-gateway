package main

import (
	"log"
	"net/http"

	"github.com/ovy95/llm-gateway/internal/api"
)

func main() {

	validKeys := map[string]string{
		"sk-demo-alice": "alice",
		"sk-demo-bob":   "bob",
	}
	a := api.New(validKeys) // build an instance, inject the keys

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.Health)
	mux.HandleFunc("POST /v1/chat/completions", a.ChatCompletions)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
