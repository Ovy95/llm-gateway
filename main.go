package main

import (
	"log"
	"net/http"
	"strings"
)

func main() {

	validKeys := map[string]string{
		"sk-demo-alice": "alice",
		"sk-demo-bob":   "bob",
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

		name, found := validKeys[key]

		if !ok || !found {
			http.Error(w, "missing or invalid api key", http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("authorized: " + name))
	})

	log.Fatal(http.ListenAndServe(":8080", mux))

}
