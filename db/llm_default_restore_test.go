package db

import "testing"

// snapshotActiveLLMProfile records which profile(s) currently carry is_default
// and schedules a cleanup that puts them back.
//
// Any test that calls SetActiveProfile MUST use this first. SetActiveProfile
// enforces the single-default invariant by running
//
//	UPDATE llm_profiles SET is_default=false WHERE is_default
//
// before flipping its own row on. A test that only deletes the throwaway
// profile it created therefore leaves the operator's real default
// deactivated — and since ActiveProfile() then returns nil, every
// LLM-dependent path (goal seeding, intercept judging, intent review)
// silently degrades for everyone sharing the dev database. That is exactly
// what happened while validating the review-auto fix: id=267 was set active
// through the API, a full `go test ./db/ ./server/` run reset it to false,
// and the only symptom was "自动处置又不工作了".
//
// Restoring in cleanup is the fix. Keep the snapshot even when the test
// expects to delete its own profile: the ids captured here are the ones that
// existed before the test ran, so restoring them cannot resurrect test rows.
func snapshotActiveLLMProfile(t *testing.T, d *DB) {
	t.Helper()
	var ids []int64
	rows, err := d.Query(`SELECT id FROM llm_profiles WHERE is_default`)
	if err != nil {
		t.Fatalf("snapshot active llm profile: %v", err)
	}
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
		// Clear first so the partial unique index (uq_llm_one_default) cannot
		// reject the restore when the test left its own default row behind.
		if _, err := d.Exec(`UPDATE llm_profiles SET is_default=false WHERE is_default`); err != nil {
			t.Errorf("restore active llm profile: clear defaults: %v", err)
			return
		}
		for _, id := range ids {
			if _, err := d.Exec(`UPDATE llm_profiles SET is_default=true WHERE id=$1`, id); err != nil {
				t.Errorf("restore active llm profile id=%d: %v", id, err)
			}
		}
	})
}
