package db

import "testing"

// TestSnapshotActiveLLMProfileRestores pins the helper that every
// SetActiveProfile-calling test must use.
//
// The pattern under test: snapshot the current default, hand the database to a
// block that flips the default somewhere else, and confirm the original is back
// once that block's cleanup has run. If the helper ever regresses to
// "clear on the way out", this fails — and that regression is what silently
// disables intent review for everyone on the shared dev database, because
// ActiveProfile() returning nil kills the third fallback level in
// reviewProvider().
func TestSnapshotActiveLLMProfileRestores(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })

	// Establish a known default: the one we will prove comes back.
	original, err := d.SaveProfile(&LLMProfile{Name: "t-restore-original", Format: "openai", Model: "m", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, original) })
	// SetActiveProfile wipes every other is_default, so snapshot what was there
	// before we take over — otherwise this test becomes the very polluter it
	// exists to catch.
	snapshotActiveLLMProfile(t, d)
	if err := d.SetActiveProfile(original); err != nil {
		t.Fatal(err)
	}

	isDefault := func(id int64) bool {
		t.Helper()
		var def bool
		if err := d.QueryRow(`SELECT is_default FROM llm_profiles WHERE id=$1`, id).Scan(&def); err != nil {
			t.Fatalf("read is_default id=%d: %v", id, err)
		}
		return def
	}
	if !isDefault(original) {
		t.Fatal("setup: original profile is not the default")
	}

	// Subtest flips the default to a throwaway row and cleans up only that row,
	// exactly like a careless caller. The snapshot is what must save us.
	t.Run("flip and clean up only own row", func(t *testing.T) {
		snapshotActiveLLMProfile(t, d)
		throwaway, err := d.SaveProfile(&LLMProfile{Name: "t-restore-throwaway", Format: "openai", Model: "m", APIKey: "k"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, throwaway) })
		if err := d.SetActiveProfile(throwaway); err != nil {
			t.Fatal(err)
		}
		if isDefault(original) {
			t.Fatal("setup: activating the throwaway profile should have cleared the original")
		}
	})

	// Subtest cleanups have run by now.
	if !isDefault(original) {
		t.Fatal("snapshotActiveLLMProfile did not restore the original default:" +
			" the next full `go test ./db/ ./server/` would leave the operator's" +
			" real LLM profile deactivated and intent review silently dead")
	}
	act, err := d.ActiveProfile()
	if err != nil {
		t.Fatal(err)
	}
	if act == nil || act.ID != original {
		t.Fatalf("ActiveProfile() = %+v, want id=%d", act, original)
	}
}
