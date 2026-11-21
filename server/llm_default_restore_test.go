package server

import (
	"testing"

	"artex/db"
)

// snapshotActiveLLMProfile records which profile(s) currently carry is_default
// and schedules a cleanup that puts them back.
//
// Any test that calls SetActiveProfile MUST use this first. SetActiveProfile
// enforces the single-default invariant by running
//
//	UPDATE llm_profiles SET is_default=false WHERE is_default
//
// before flipping its own row on, so a test that cleans up only its own
// throwaway profile leaves the operator's real default deactivated. Because
// ActiveProfile() then returns nil, reviewProvider loses its third fallback
// level and the intent-review round goes permanently silent — the exact
// symptom the review-auto fix was supposed to cure.
//
// Order matters in the cleanup: clear every default before restoring the
// snapshot, otherwise the partial unique index uq_llm_one_default rejects the
// restore when the test left its own default row in place.
func snapshotActiveLLMProfile(t *testing.T, pg *db.DB) {
	t.Helper()

	rows, err := pg.Query(`SELECT id FROM llm_profiles WHERE is_default`)
	if err != nil {
		t.Fatalf("snapshot active llm profile: %v", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("scan active llm profile id: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("snapshot active llm profile: %v", err)
	}

	t.Cleanup(func() {
		if _, err := pg.Exec(`UPDATE llm_profiles SET is_default=false WHERE is_default`); err != nil {
			t.Errorf("restore active llm profile: clear defaults: %v", err)
			return
		}
		for _, id := range ids {
			if _, err := pg.Exec(`UPDATE llm_profiles SET is_default=true WHERE id=$1`, id); err != nil {
				t.Errorf("restore active llm profile id=%d: %v", id, err)
			}
		}
	})
}
