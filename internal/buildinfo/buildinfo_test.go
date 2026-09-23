package buildinfo

import "testing"

func TestCurrentPreservesInjectedReleaseMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = originalVersion, originalCommit, originalDate })
	Version, Commit, Date = "1.2.3", "abc123", "2026-09-24T00:00:00Z"
	got := Current()
	if got.Version != Version || got.Commit != Commit || got.Date != Date {
		t.Fatalf("Current() = %+v", got)
	}
}
