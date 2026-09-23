package moe

import (
	"errors"
	"testing"
)

func TestDelegationLeaseStoreRejectsStaleCheckpoint(t *testing.T) {
	lease := NewDelegationLease("run-cas", DelegationLimits{})
	snapshot, err := lease.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryDelegationLeaseStore()
	created, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	next := cloneDelegationSnapshot(snapshot)
	next.Nodes = 1
	updated, err := store.CompareAndSwap(snapshot.RunID, created.Revision, next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Snapshot.Nodes != 1 {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if _, err := store.CompareAndSwap(snapshot.RunID, created.Revision, snapshot); !errors.Is(err, ErrLeaseRevisionConflict) {
		t.Fatalf("stale checkpoint error = %v", err)
	}
}

func TestDelegationLeaseStoreReturnsDeepCopies(t *testing.T) {
	snapshot, err := NewDelegationLease("run-copy", DelegationLimits{}).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryDelegationLeaseStore()
	created, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	created.Snapshot.Children["mutated"] = 1
	loaded, ok, err := store.Load(snapshot.RunID)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if loaded.Snapshot.Children["mutated"] != 0 {
		t.Fatal("stored snapshot aliased caller-owned map")
	}
}
