package tui

import "testing"

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		input, target string
		match         bool
	}{
		{"/co", "/commit", true},
		{"/co", "/compact", true},
		{"/cm", "/commit", true},
		{"/cm", "/help", false},
		{"/", "/help", true},
		{"/he", "/help", true},
		{"/hl", "/help", true},
		{"/xz", "/exit", false},
	}
	for _, tt := range tests {
		t.Run(tt.input+"→"+tt.target, func(t *testing.T) {
			got, _ := fuzzyMatchCommand(tt.input, tt.target)
			if got != tt.match {
				t.Errorf("fuzzyMatchCommand(%q, %q) = %v, want %v", tt.input, tt.target, got, tt.match)
			}
		})
	}
}

func TestFuzzyMatchScore(t *testing.T) {
	_, prefixScore := fuzzyMatchCommand("/co", "/cost")
	_, subseqScore := fuzzyMatchCommand("/ct", "/context")
	if prefixScore <= subseqScore {
		t.Errorf("prefix score (%d) should > subsequence score (%d)", prefixScore, subseqScore)
	}
}
