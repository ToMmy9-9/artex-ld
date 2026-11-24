package llmrec

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"artex/db"
	"github.com/google/uuid"
	"norma/llm"
	"norma/transcript"
)

// realMiniMaxFrames drives a real Anthropic adapter against SSE frames copied
// verbatim from a production response, so this test exercises the same parse
// path the live system uses rather than a hand-built fake provider.
func realMiniMaxFrames(t *testing.T) llm.Provider {
	t.Helper()
	body, err := os.ReadFile("../third_party/norma/llm/testdata/minimax_real_usage_frames.sse")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	p, err := llm.NewProvider(llm.Config{
		Format:  llm.FormatAnthropic,
		BaseURL: srv.URL,
		APIKey:  "k",
		Model:   "m",
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return p
}

// TestUsagePersistedFromRealWireFrames closes the loop that the unit test opens:
// the fix lives in the adapter's stream parser, but the number that matters is
// what lands in the usage ledger. A production row for these exact frames was
// stored as input_tokens=0 while the wire carried 2661 + 120448 = 123109.
func TestUsagePersistedFromRealWireFrames(t *testing.T) {
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated ARTEX_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	profile := "usage-persist-" + uuid.NewString()
	ctx := transcript.WithSessionID(t.Context(), "exp0-usage-test")
	recorder := Wrap(realMiniMaxFrames(t), pg, "fixture", profile, "", "", func() bool { return false })

	var sawErr error
	for _, err := range recorder.Stream(ctx, llm.CompletionRequest{}) {
		if err != nil {
			sawErr = err
			break
		}
	}

	var count, input, output, cacheRead int
	err = pg.QueryRow(`SELECT count(*), max(input_tokens), max(output_tokens), max(cache_read)
FROM llm_usage WHERE profile_name=$1`, profile).Scan(&count, &input, &output, &cacheRead)
	_, _ = pg.Exec(`DELETE FROM llm_usage WHERE profile_name=$1`, profile)
	_, _ = pg.Exec(`DELETE FROM llm_records WHERE profile_name=$1`, profile)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	// A stream error would still write a row, but the usage check below is
	// meaningless then — surface the wire error instead of a confusing mismatch.
	if sawErr != nil {
		t.Fatalf("stream error: %v", sawErr)
	}
	if count != 1 {
		t.Fatalf("usage rows = %d, want 1", count)
	}
	// These are the numbers actually on the wire for these frames; before the
	// adapter fix, input and cache_read both persisted as 0.
	if input != 123109 || output != 15470 || cacheRead != 120448 {
		t.Fatalf("persisted usage in=%d out=%d cache_read=%d, want 123109/15470/120448", input, output, cacheRead)
	}
}

// TestUsagePersistCountsEveryProvider1 calls Stream twice to make sure the
// de-duplication state in the adapter is per-stream, not per-provider — a shared
// flag would silently zero the input side of the second call.
func TestUsagePersistCountsEveryProvider(t *testing.T) {
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated ARTEX_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	prov := realMiniMaxFrames(t)
	for i := 0; i < 2; i++ {
		profile := "usage-repeat-" + uuid.NewString()
		ctx := transcript.WithSessionID(t.Context(), "exp0-usage-test")
		recorder := Wrap(prov, pg, "fixture", profile, "", "", func() bool { return false })
		for _, err := range recorder.Stream(ctx, llm.CompletionRequest{}) {
			if err != nil {
				t.Fatalf("iteration %d: %v", i, err)
			}
		}
		var input int
		if err := pg.QueryRow(`SELECT max(input_tokens) FROM llm_usage WHERE profile_name=$1`, profile).Scan(&input); err != nil {
			t.Fatalf("iteration %d query: %v", i, err)
		}
		_, _ = pg.Exec(`DELETE FROM llm_usage WHERE profile_name=$1`, profile)
		_, _ = pg.Exec(`DELETE FROM llm_records WHERE profile_name=$1`, profile)
		if input != 123109 {
			t.Fatalf("iteration %d persisted input=%d, want 123109", i, input)
		}
	}
}
