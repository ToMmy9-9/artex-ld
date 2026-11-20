package server

import (
	"context"
	"strings"
	"testing"

	"artex/db"
)

// TestChatUnavailableReasonDistinguishesStates pins the operator-facing message
// to the actual configuration state: nothing configured vs. configured-but-not-
// activated must read differently so the user knows what to do.
//
// This exercises llmUnavailableReason directly. It used to spin up a manager and
// DELETE EVERY ROW of llm_profiles to construct the "nothing configured" state
// — on a shared dev database that wiped the operator's own LLM configurations
// (and with them the model judge behind intercept approvals). The mapping is a
// pure function, so there is no reason to reach into a live table at all.
func TestChatUnavailableReasonDistinguishesStates(t *testing.T) {
	if got := llmUnavailableReason(0, false); !strings.Contains(got, "尚未配置") {
		t.Fatalf("no-profile reason=%q, want 尚未配置", got)
	}

	// Profile exists but is not activated.
	if got := llmUnavailableReason(1, false); !strings.Contains(got, "没有已激活") {
		t.Fatalf("inactive reason=%q, want 没有已激活", got)
	}

	// Once activated, the reason no longer claims a missing/inactive config.
	if got := llmUnavailableReason(1, true); strings.Contains(got, "尚未配置") || strings.Contains(got, "没有已激活") {
		t.Fatalf("active reason=%q should not report missing/inactive", got)
	}

	// A profile table with several entries but none active is still "inactive",
	// not "unconfigured" — the distinction is what the message is for.
	if got := llmUnavailableReason(3, false); !strings.Contains(got, "没有已激活") {
		t.Fatalf("multi-inactive reason=%q, want 没有已激活", got)
	}
}

// TestResolveChatAgentHonoursConversationProfile is the regression guard for the
// reported bug: a conversation that picked a valid profile must resolve a chat
// agent even when NO global profile is active, so the send precheck stops
// rejecting it with "LLM 未配置".
func TestResolveChatAgentHonoursConversationProfile(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { m.Close() })
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)

	// Force the global fallback to nil so a non-nil result can ONLY come from the
	// conversation's own profile — this is exactly the situation the user hit
	// (no usable global config) and isolates the fix from the test host's env.
	s.cfgMu.Lock()
	s.chatAgent = nil
	s.cfgMu.Unlock()

	id, err := m.pg.SaveProfile(&db.LLMProfile{Name: "conv-pick", Format: "anthropic", Model: "claude-x", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.pg.DeleteProfile(id) })

	// A conversation that pinned this profile resolves a chat agent despite the
	// nil global fallback; without the fix the send precheck rejected it outright.
	conv := &db.Conversation{AgentKey: "mainagent", LLMProfileID: &id}
	if s.resolveChatAgent(conv) == nil {
		t.Fatalf("resolveChatAgent(conversation with valid profile) = nil; global fallback wrongly gates the pick")
	}

	// With neither a pick nor a global fallback there is genuinely nothing to run.
	if s.resolveChatAgent(&db.Conversation{AgentKey: "mainagent"}) != nil {
		t.Fatalf("resolveChatAgent with neither pick nor global config should be nil")
	}
}
