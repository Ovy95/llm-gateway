package api

import (
	"net/http"
	"strings"
)

type API struct {
	validKeys map[string]string
}

func New(keys map[string]string) *API {
	return &API{validKeys: keys}
}

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("ok"))
	if err != nil {
		return
	}
}

func (a *API) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	name, found := a.validKeys[key]

	if !ok || !found {
		http.Error(w, "missing or invalid api key", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("authorized: " + name))
}
