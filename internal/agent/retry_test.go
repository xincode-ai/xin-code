package agent

import (
	"testing"
	"time"
)

func TestCalcRetryDelay_ExponentialBackoff(t *testing.T) {
	// attempt 1: 500ms base
	d1 := CalcRetryDelay(1, "")
	if d1 < 375*time.Millisecond || d1 > 625*time.Millisecond {
		t.Errorf("attempt 1 delay should be ~500ms±25%%, got %v", d1)
	}

	// attempt 3: 2000ms base
	d3 := CalcRetryDelay(3, "")
	if d3 < 1500*time.Millisecond || d3 > 2500*time.Millisecond {
		t.Errorf("attempt 3 delay should be ~2000ms±25%%, got %v", d3)
	}

	// cap at 32s
	d10 := CalcRetryDelay(10, "")
	if d10 > 40*time.Second {
		t.Errorf("delay should be capped, got %v", d10)
	}
}

func TestCalcRetryDelay_RetryAfterHeader(t *testing.T) {
	d := CalcRetryDelay(1, "5")
	if d < 5*time.Second {
		t.Errorf("should respect Retry-After header, got %v", d)
	}
}

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{429, true},
		{529, true},
		{500, true},
		{502, true},
		{503, true},
		{408, true},
		{400, false},
		{404, false},
		{200, false},
	}
	for _, tt := range tests {
		if IsRetryableStatusCode(tt.code) != tt.retryable {
			t.Errorf("status %d: expected retryable=%v", tt.code, tt.retryable)
		}
	}
}
