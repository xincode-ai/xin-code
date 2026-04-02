package provider

import "testing"

func TestResolveProviderName(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-sonnet-4-6", "anthropic"},
		{"claude-opus-4", "anthropic"},
		{"sonnet-4-6", "anthropic"},
		{"haiku-4-5", "anthropic"},
		{"gpt-4o", "openai"},
		{"o1-preview", "openai"},
		{"o3-mini", "openai"},
		{"deepseek-chat", "openai"},  // 兜底
		{"unknown-model", "openai"},  // 兜底
	}
	for _, tt := range tests {
		got := ResolveProviderName(tt.model)
		if got != tt.expected {
			t.Errorf("ResolveProviderName(%q) = %q, want %q", tt.model, got, tt.expected)
		}
	}
}
