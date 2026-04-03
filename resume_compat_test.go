package main

import (
	"testing"

	"github.com/xincode-ai/xin-code/internal/session"
	"github.com/xincode-ai/xin-code/internal/tui"
)

func TestClassifyResumeCompat(t *testing.T) {
	tests := []struct {
		name       string
		entry      session.IndexEntry
		curProv    string
		curBaseURL string
		curAuth    string
		wantMode   tui.ResumeMode
	}{
		{
			name:     "同 provider + 同 baseURL + 同 authSource → continue",
			entry:    session.IndexEntry{Model: "claude-opus-4-6", Provider: "anthropic", BaseURL: "", AuthSource: "config"},
			curProv:  "anthropic", curBaseURL: "", curAuth: "config",
			wantMode: tui.ResumeContinue,
		},
		{
			name:     "同 provider + 同 baseURL + 不同 authSource → readonly",
			entry:    session.IndexEntry{Model: "claude-opus-4-6", Provider: "anthropic", BaseURL: "", AuthSource: "cc-oauth"},
			curProv:  "anthropic", curBaseURL: "", curAuth: "config",
			wantMode: tui.ResumeReadonly,
		},
		{
			name:     "跨 provider → blocked",
			entry:    session.IndexEntry{Model: "gpt-4", Provider: "openai"},
			curProv:  "anthropic", curBaseURL: "", curAuth: "config",
			wantMode: tui.ResumeBlocked,
		},
		{
			name:     "跨 endpoint → blocked",
			entry:    session.IndexEntry{Model: "claude-opus-4-6", Provider: "anthropic", BaseURL: "https://custom.api.com"},
			curProv:  "anthropic", curBaseURL: "", curAuth: "config",
			wantMode: tui.ResumeBlocked,
		},
		{
			name:     "旧会话无 Provider/AuthSource → continue（降级推断）",
			entry:    session.IndexEntry{Model: "claude-opus-4-6", Provider: "", BaseURL: "", AuthSource: ""},
			curProv:  "anthropic", curBaseURL: "", curAuth: "config",
			wantMode: tui.ResumeContinue,
		},
		{
			name:     "旧会话无 authSource + 当前有 → continue（空视为兼容）",
			entry:    session.IndexEntry{Model: "claude-opus-4-6", Provider: "anthropic", BaseURL: "", AuthSource: ""},
			curProv:  "anthropic", curBaseURL: "", curAuth: "cc-oauth",
			wantMode: tui.ResumeContinue,
		},
		{
			name:     "同 provider 不同 model → continue（同 provider 内可切模型）",
			entry:    session.IndexEntry{Model: "claude-sonnet-4-6", Provider: "anthropic"},
			curProv:  "anthropic", curBaseURL: "", curAuth: "",
			wantMode: tui.ResumeContinue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, reason := classifyResumeCompat(tt.entry, tt.curProv, tt.curBaseURL, tt.curAuth)
			if mode != tt.wantMode {
				t.Errorf("mode = %q (reason: %q), want %q", mode, reason, tt.wantMode)
			}
			if reason == "" {
				t.Error("reason 不应为空")
			}
		})
	}
}
