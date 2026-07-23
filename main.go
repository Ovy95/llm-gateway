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
	a := api.New(validKeys)

	log.Fatal(http.ListenAndServe(":8080", a.Routes()))
}
