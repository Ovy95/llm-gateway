package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ovy95/llm-gateway/internal/api"
)

func TestStats(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"gpt-4o-mini","usage":{"prompt_tokens":10,"completion_tokens":20}}`))
	}))
	defer fake.Close()

	h := api.New(api.Config{
		Keys: map[string]api.KeyConfig{
			"sk-demo-alice": {Tenant: "alice", BudgetUSD: 0.10, RPM: 100},
			"sk-demo-bob":   {Tenant: "bob", BudgetUSD: 0.01, RPM: 100},
		},
		UpstreamKey: "fake-key",
		UpstreamURL: fake.URL,
		Client:      fake.Client(),
	}).Routes()

	doRequest(t, h, "sk-demo-alice")
	doRequest(t, h, "sk-demo-alice")

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("stats: got status %d, want 200", w.Code)
	}

	var stats map[string]struct {
		Requests int `json:"requests"`
		Tokens   int `json:"tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decoding stats: %v", err)
	}

	tests := []struct {
		name         string
		tenant       string
		wantRequests int
		wantTokens   int
	}{
		{name: "happy path: alice's two calls are recorded", tenant: "alice", wantRequests: 2, wantTokens: 60},
		{name: "happy path: bob with no calls reads zero", tenant: "bob", wantRequests: 0, wantTokens: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stats[tt.tenant]
			if got.Requests != tt.wantRequests {
				t.Errorf("%s requests = %d, want %d", tt.tenant, got.Requests, tt.wantRequests)
			}
			if got.Tokens != tt.wantTokens {
				t.Errorf("%s tokens = %d, want %d", tt.tenant, got.Tokens, tt.wantTokens)
			}
		})
	}
}
