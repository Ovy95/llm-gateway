package api_test

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ovy95/llm-gateway/internal/api"
)

// fakeStore records every call so a test can assert exactly what the handler recorded.
type fakeStore struct {
	records []record
}
type record struct {
	key    string
	tokens int
	cost   float64
}

func (f *fakeStore) Record(key string, tokens int, costUSD float64) {
	f.records = append(f.records, record{key, tokens, costUSD})
}
func (f *fakeStore) SpendUSD(key string) float64        { return 0 }
func (f *fakeStore) CurrentUsage() map[string]api.Usage { return nil }

func TestChatCompletions_RecordsUsageThroughStore(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"gpt-4o-mini","usage":{"prompt_tokens":10,"completion_tokens":20}}`))
	}))
	defer fake.Close()

	store := &fakeStore{}
	h := api.New(api.Config{
		Keys:        map[string]api.KeyConfig{"sk-demo-alice": {Tenant: "alice", BudgetUSD: 100, RPM: 100}},
		UpstreamKey: "fake-key",
		UpstreamURL: fake.URL,
		Client:      fake.Client(),
		Store:       store,
	}).Routes()

	doRequest(t, h, "sk-demo-alice")

	if len(store.records) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(store.records))
	}
	got := store.records[0]
	if got.key != "sk-demo-alice" {
		t.Errorf("recorded key = %q, want sk-demo-alice", got.key)
	}
	if got.tokens != 30 {
		t.Errorf("recorded tokens = %d, want 30", got.tokens)
	}
	wantCost := 0.15*10/1_000_000 + 0.60*20/1_000_000 // 0.0000135
	if math.Abs(got.cost-wantCost) > 1e-9 {
		t.Errorf("recorded cost = %v, want %v", got.cost, wantCost)
	}
}
