package llm

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// usageFromStream runs one stream against a canned SSE body and returns the
// accumulated usage, mirroring exactly how the harness consumes it.
func usageFromStream(t *testing.T, frames []string) Usage {
	t.Helper()
	body := strings.Join(frames, "\n\n") + "\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := NewProvider(Config{Format: FormatAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	acc := NewAccumulator()
	stream := p.Stream(context.Background(), CompletionRequest{Messages: []Message{UserText("hi")}})
	for ev, e := range stream {
		if e != nil {
			t.Fatalf("stream err: %v", e)
		}
		acc.Add(ev)
	}
	return acc.Usage
}

func sseFrame(event, data string) string {
	return "event: " + event + "\ndata: " + data
}

// MiniMax-style gateway: message_start reports input_tokens=0, and only
// message_delta carries the real input side. Production measured this shape on
// 3460/3470 rows with input_tokens stuck at 0 — dropping the delta's input side
// loses the entire measurement, which then makes the compaction threshold read 0
// and never fire.
func TestStreamUsageInputOnlyInMessageDelta(t *testing.T) {
	got := usageFromStream(t, []string{
		sseFrame("message_start", `{"type":"message_start","message":{"usage":{"input_tokens":0,"output_tokens":1}}}`),
		sseFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`),
		sseFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":6415,"output_tokens":308,"cache_read_input_tokens":73319}}`),
		sseFrame("message_stop", `{"type":"message_stop"}`),
	})
	// InputTokens is the TOTAL input including cached (OpenAI prompt_tokens
	// semantics), so 6415 + 73319 = 79734.
	want := Usage{InputTokens: 79734, OutputTokens: 308, CacheReadTokens: 73319}
	if got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
}

// Same assertion as above, but driven by frames copied verbatim out of a real
// production response (llm_records.id=31300, MiniMax-M3.1-Flash, 2026-11-24).
// That row was stored as input_tokens=0 while the wire actually carried
// 2661 + 120448 = 123109 input tokens — the regression this fix exists for.
func TestStreamUsageFromRealProductionFrames(t *testing.T) {
	body, err := os.ReadFile("testdata/minimax_real_usage_frames.sse")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p, err := NewProvider(Config{Format: FormatAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	acc := NewAccumulator()
	for ev, e := range p.Stream(context.Background(), CompletionRequest{Messages: []Message{UserText("hi")}}) {
		if e != nil {
			t.Fatalf("stream err: %v", e)
		}
		acc.Add(ev)
	}
	want := Usage{InputTokens: 123109, OutputTokens: 15470, CacheReadTokens: 120448}
	if acc.Usage != want {
		t.Fatalf("usage = %+v, want %+v (was silently 0 before the fix)", acc.Usage, want)
	}
}

// Anthropic official: message_start reports the input side and message_delta
// echoes it. Forwarding both would double the input and cache totals.
func TestStreamUsageInputEchoedInDeltaIsNotDoubled(t *testing.T) {
	got := usageFromStream(t, []string{
		sseFrame("message_start", `{"type":"message_start","message":{"usage":{"input_tokens":150,"output_tokens":1,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}}`),
		sseFrame("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`),
		sseFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":150,"output_tokens":42,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}}`),
		sseFrame("message_stop", `{"type":"message_stop"}`),
	})
	want := Usage{InputTokens: 1100, OutputTokens: 42, CacheReadTokens: 900, CacheWriteTokens: 50}
	if got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
}

// A gateway that omits message_start usage entirely: the delta's input side is
// the only source and must survive.
func TestStreamUsageNoMessageStartUsage(t *testing.T) {
	got := usageFromStream(t, []string{
		sseFrame("message_start", `{"type":"message_start","message":{}}`),
		sseFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1234,"output_tokens":56}}`),
		sseFrame("message_stop", `{"type":"message_stop"}`),
	})
	want := Usage{InputTokens: 1234, OutputTokens: 56}
	if got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
}

// The non-streaming path shares norm(), so it must report the same semantics.
func TestParseAnthropicResponseUsage(t *testing.T) {
	raw := []byte(`{"id":"x","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":6415,"output_tokens":308,"cache_read_input_tokens":73319}}`)
	_, _, usage, err := parseAnthropicResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := Usage{InputTokens: 79734, OutputTokens: 308, CacheReadTokens: 73319}
	if usage != want {
		t.Fatalf("usage = %+v, want %+v", usage, want)
	}
}

// Guard the shape the harness actually depends on: a stream that reports input
// only at message_delta must still yield a non-zero total, otherwise
// lastInputTokens stays 0 and compaction never triggers.
func TestStreamUsageTotalIsNonZeroForRealisticTurn(t *testing.T) {
	var events []StreamEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, f := range []string{
			sseFrame("message_start", `{"type":"message_start","message":{"usage":{"input_tokens":0,"output_tokens":1}}}`),
			sseFrame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":52000,"output_tokens":700}}`),
			sseFrame("message_stop", `{"type":"message_stop"}`),
		} {
			_, _ = w.Write([]byte(f + "\n\n"))
		}
	}))
	defer srv.Close()

	p, err := NewProvider(Config{Format: FormatAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var seq iter.Seq2[StreamEvent, error] = p.Stream(context.Background(), CompletionRequest{Messages: []Message{UserText("hi")}})
	for ev, e := range seq {
		if e != nil {
			t.Fatal(e)
		}
		events = append(events, ev)
	}
	if len(events) == 0 {
		t.Fatal("no events")
	}
	acc := NewAccumulator()
	for _, ev := range events {
		acc.Add(ev)
	}
	if acc.Usage.InputTokens != 52000 {
		t.Fatalf("accumulated input = %d, want 52000 (compaction gate reads this)", acc.Usage.InputTokens)
	}
}
