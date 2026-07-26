package api_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ovy95/llm-gateway/internal/api"
)

func TestChatCompletions_Auth(t *testing.T) {
	upstreamHit := false
	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		w.Write([]byte(`{"ok":true}`))
	}))
	defer fakeUpstream.Close()

	a := api.New(api.Config{
		Keys:        map[string]api.KeyConfig{"sk-demo-alice": {Tenant: "alice", BudgetUSD: 100, RPM: 5}},
		UpstreamKey: "fake-key",
		UpstreamURL: fakeUpstream.URL,
		Client:      fakeUpstream.Client(),
	})

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantHit    bool
	}{
		{
			name:       "sad path: no authorization header returns 401 and never reaches upstream",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantHit:    false,
		},
		{
			name:       "sad path: unrecognised key returns 401 and never reaches upstream",
			authHeader: "Bearer wrong-key",
			wantStatus: http.StatusUnauthorized,
			wantHit:    false,
		},
		{
			name:       "happy path: valid key is forwarded and returns the upstream response",
			authHeader: "Bearer sk-demo-alice",
			wantStatus: http.StatusOK,
			wantHit:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstreamHit = false // reset — shared across subtests, easy to forget

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			a.Routes().ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
			if upstreamHit != tt.wantHit {
				t.Errorf("upstream hit = %v, want %v", upstreamHit, tt.wantHit)
			}
		})
	}
}

func TestChatCompletions_UpstreamFailures(t *testing.T) {
	tests := []struct {
		name       string
		upstream   http.HandlerFunc
		timeout    time.Duration
		wantStatus int
	}{
		{
			name: "happy path: upstream 200 is passed through to the client",
			upstream: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"ok":true}`))
			},
			timeout:    api.DefaultUpstreamTimeout,
			wantStatus: http.StatusOK,
		},
		{
			name: "sad path: upstream 500 is passed through as 500",
			upstream: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"boom"}`))
			},
			timeout:    api.DefaultUpstreamTimeout,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "sad path: upstream slower than the deadline returns 504",
			upstream: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				w.Write([]byte(`{"ok":true}`))
			},
			timeout:    10 * time.Millisecond,
			wantStatus: http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := httptest.NewServer(tt.upstream)
			defer fake.Close()

			a := api.New(api.Config{
				Keys:        map[string]api.KeyConfig{"sk-demo-alice": {Tenant: "alice", BudgetUSD: 100, RPM: 5}},
				UpstreamKey: "fake-key",
				UpstreamURL: fake.URL,
				Client:      fake.Client(),
				Timeout:     tt.timeout,
			})

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Header.Set("Authorization", "Bearer sk-demo-alice")
			w := httptest.NewRecorder()

			a.Routes().ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func doRequest(t *testing.T, h http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestChatCompletions_RateLimit(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()

	tests := []struct {
		name           string
		priorRequests  []string // keys fired before the request under test
		key            string   // the request we actually assert on
		wantStatus     int
		wantRetryAfter bool
	}{
		{
			name:          "happy path: 5th request within the burst is allowed",
			priorRequests: []string{"sk-demo-alice", "sk-demo-alice", "sk-demo-alice", "sk-demo-alice"},
			key:           "sk-demo-alice",
			wantStatus:    http.StatusOK,
		},
		{
			name:           "sad path: 6th request past the burst is rate limited with Retry-After",
			priorRequests:  []string{"sk-demo-alice", "sk-demo-alice", "sk-demo-alice", "sk-demo-alice", "sk-demo-alice"},
			key:            "sk-demo-alice",
			wantStatus:     http.StatusTooManyRequests,
			wantRetryAfter: true,
		},
		{
			name:          "happy path: one key hitting its limit does not affect another key",
			priorRequests: []string{"sk-demo-alice", "sk-demo-alice", "sk-demo-alice", "sk-demo-alice", "sk-demo-alice", "sk-demo-alice"},
			key:           "sk-demo-bob",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "sad path: bob's lower 2/min limit blocks his 3rd request",
			priorRequests: []string{"sk-demo-bob", "sk-demo-bob"},
			key:           "sk-demo-bob",
			wantStatus:    http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := api.New(api.Config{
				Keys:        map[string]api.KeyConfig{"sk-demo-alice": {Tenant: "alice", BudgetUSD: 100, RPM: 5}, "sk-demo-bob": {Tenant: "bob", BudgetUSD: 100, RPM: 2}},
				UpstreamKey: "fake-key",
				UpstreamURL: fake.URL,
				Client:      fake.Client(),
			}).Routes()

			for _, k := range tt.priorRequests {
				doRequest(t, h, k)
			}

			w := doRequest(t, h, tt.key)
			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantRetryAfter && w.Header().Get("Retry-After") == "" {
				t.Errorf("expected Retry-After header on 429, got none")
			}
		})
	}
}

func TestChatCompletions_SpendCap(t *testing.T) {
	// each call reports 1M+1M tokens on gpt-4o-mini = $0.15 + $0.60 = $0.75
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"gpt-4o-mini","Usage":{"prompt_tokens":1000000,"completion_tokens":1000000}}`))
	}))
	defer fake.Close()

	tests := []struct {
		name       string
		priorCalls int
		wantStatus int
	}{
		{
			name:       "happy path: first request under budget is allowed",
			priorCalls: 0,
			wantStatus: http.StatusOK,
		},
		{
			name:       "sad path: request after the budget is spent is rejected with 402",
			priorCalls: 1, // one $0.75 call already blows the $0.10 budget
			wantStatus: http.StatusPaymentRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := api.New(api.Config{
				Keys:        map[string]api.KeyConfig{"sk-demo-alice": {Tenant: "alice", BudgetUSD: 0.10, RPM: 100}},
				UpstreamKey: "fake-key",
				UpstreamURL: fake.URL,
				Client:      fake.Client(),
			}).Routes()

			for i := 0; i < tt.priorCalls; i++ {
				doRequest(t, h, "sk-demo-alice")
			}

			w := doRequest(t, h, "sk-demo-alice")
			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestChatCompletions_AuditLogNoBodies(t *testing.T) {
	const secretPrompt = "SUPER_SECRET_PROMPT_12345"
	const secretResponse = "SUPER_SECRET_RESPONSE_67890"

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"gpt-4o-mini","Usage":{"prompt_tokens":10,"completion_tokens":20},"choices":[{"message":{"content":"` + secretResponse + `"}}]}`))
	}))
	defer fake.Close()

	var logBuf bytes.Buffer
	a := api.New(api.Config{
		Keys:        map[string]api.KeyConfig{"sk-demo-alice": {Tenant: "alice", BudgetUSD: 100, RPM: 100}},
		UpstreamKey: "fake-key",
		UpstreamURL: fake.URL,
		Client:      fake.Client(),
		Logger:      slog.New(slog.NewJSONHandler(&logBuf, nil)),
	})

	reqBody := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"` + secretPrompt + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-demo-alice")
	w := httptest.NewRecorder()
	a.Routes().ServeHTTP(w, req)

	logOutput := logBuf.String()

	tests := []struct {
		name    string
		content string
	}{
		{name: "sad path: request prompt content must not appear in the audit log", content: secretPrompt},
		{name: "sad path: response body content must not appear in the audit log", content: secretResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(logOutput, tt.content) {
				t.Errorf("audit log leaked content %q; log was: %s", tt.content, logOutput)
			}
		})
	}

	t.Run("happy path: audit log contains the structured metadata fields", func(t *testing.T) {
		for _, field := range []string{"decision", "cost_usd", "model", "latency_ms"} {
			if !strings.Contains(logOutput, field) {
				t.Errorf("audit log missing expected field %q; log was: %s", field, logOutput)
			}
		}
	})
}
