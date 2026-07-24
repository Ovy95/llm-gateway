package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
