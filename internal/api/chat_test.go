package api_test

import (
	"net/http"
	"net/http/httptest"
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
		Keys:        map[string]string{"sk-demo-alice": "alice"},
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
				Keys:        map[string]string{"sk-demo-alice": "alice"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := api.New(api.Config{
				Keys:        map[string]string{"sk-demo-alice": "alice", "sk-demo-bob": "bob"},
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
