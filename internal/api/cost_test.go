package api

import (
	"math"
	"testing"
)

func TestCostUSD(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		promptTokens     int
		completionTokens int
		wantCost         float64
		wantOK           bool
	}{
		{
			name:             "happy path: known model prices prompt and completion separately",
			model:            "gpt-4o-mini",
			promptTokens:     1000,
			completionTokens: 500,
			wantCost:         0.00045,
			wantOK:           true,
		},
		{
			name:             "happy path: zero tokens costs nothing",
			model:            "gpt-4o-mini",
			promptTokens:     0,
			completionTokens: 0,
			wantCost:         0,
			wantOK:           true,
		},
		{
			name:             "sad path: unknown model returns ok=false and zero cost",
			model:            "gpt-9-ultra",
			promptTokens:     1000,
			completionTokens: 500,
			wantCost:         0,
			wantOK:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCost, gotOK := costUSD(tt.model, tt.promptTokens, tt.completionTokens)
			if gotOK != tt.wantOK {
				t.Errorf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if math.Abs(gotCost-tt.wantCost) > 1e-9 {
				t.Errorf("cost = %v, want %v", gotCost, tt.wantCost)
			}
		})
	}
}
