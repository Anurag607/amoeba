package extension

import (
	"context"
	"errors"
	"testing"
)

func TestActivationRollsBackAndQuarantineBlocks(t *testing.T) {
	registry := NewRegistry[string]()
	rolledBack := false
	_ = registry.Register(Contribution[string]{ID: "a", Version: "1", Owner: "host", Value: "a",
		Activate:   func(context.Context, string) error { return nil },
		Deactivate: func(context.Context, string) error { rolledBack = true; return nil }})
	_ = registry.Register(Contribution[string]{ID: "b", Version: "1", Owner: "host", Value: "b", Dependencies: []string{"a"},
		Activate: func(context.Context, string) error { return errors.New("broken") }})
	if err := registry.ActivateAll(context.Background(), []string{"a", "b"}); err == nil || !rolledBack {
		t.Fatalf("activation did not roll back: %v", err)
	}
	if len(registry.ActiveSnapshot()) != 0 {
		t.Fatal("rollback left active contribution")
	}
	if err := registry.Quarantine("b", "repeated failure"); err != nil {
		t.Fatal(err)
	}
	if err := registry.ActivateAll(context.Background(), []string{"b"}); err == nil {
		t.Fatal("quarantined extension activated")
	}
}

func TestRollbackDoesNotDeactivatePreviouslyActiveExtension(t *testing.T) {
	registry := NewRegistry[string]()
	deactivated := false
	_ = registry.Register(Contribution[string]{ID: "active", Version: "1", Owner: "host", Value: "active",
		Activate: func(context.Context, string) error { return nil },
		Deactivate: func(context.Context, string) error {
			deactivated = true
			return nil
		}})
	if err := registry.ActivateAll(context.Background(), []string{"active"}); err != nil {
		t.Fatal(err)
	}
	_ = registry.Register(Contribution[string]{ID: "broken", Version: "1", Owner: "host", Value: "broken",
		Activate: func(context.Context, string) error { return errors.New("broken") }})
	if err := registry.ActivateAll(context.Background(), []string{"active", "broken"}); err == nil {
		t.Fatal("expected activation error")
	}
	if deactivated {
		t.Fatal("rollback deactivated an extension active before this transaction")
	}
}

func TestQuarantineDeactivatesActiveExtension(t *testing.T) {
	registry := NewRegistry[string]()
	deactivated := false
	_ = registry.Register(Contribution[string]{ID: "active", Version: "1", Owner: "host", Value: "active",
		Activate: func(context.Context, string) error { return nil },
		Deactivate: func(context.Context, string) error {
			deactivated = true
			return nil
		}})
	if err := registry.ActivateAll(context.Background(), []string{"active"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Quarantine("active", "health check failed"); err != nil {
		t.Fatal(err)
	}
	if !deactivated || len(registry.ActiveSnapshot()) != 0 {
		t.Fatal("quarantine left active contribution running")
	}
}

func TestActivationSortsDependenciesAndRejectsCycles(t *testing.T) {
	registry := NewRegistry[string]()
	var order []string
	_ = registry.Register(Contribution[string]{ID: "dependency", Version: "1", Owner: "host", Value: "dependency", Activate: func(context.Context, string) error { order = append(order, "dependency"); return nil }})
	_ = registry.Register(Contribution[string]{ID: "consumer", Version: "1", Owner: "host", Value: "consumer", Dependencies: []string{"dependency"}, Activate: func(context.Context, string) error { order = append(order, "consumer"); return nil }})
	if err := registry.ActivateAll(context.Background(), []string{"consumer"}); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "dependency" {
		t.Fatalf("order=%v", order)
	}
	cycles := NewRegistry[string]()
	_ = cycles.Register(Contribution[string]{ID: "a", Version: "1", Owner: "host", Dependencies: []string{"b"}})
	_ = cycles.Register(Contribution[string]{ID: "b", Version: "1", Owner: "host", Dependencies: []string{"a"}})
	if err := cycles.ActivateAll(context.Background(), []string{"a"}); err == nil {
		t.Fatal("dependency cycle activated")
	}
}

func TestHealthFailureQuarantinesAndUpdateRollsBack(t *testing.T) {
	registry := NewRegistry[string]()
	restored := false
	_ = registry.Register(Contribution[string]{ID: "a", Version: "1", Owner: "host", Value: "old",
		Activate: func(context.Context, string) error { restored = true; return nil },
		Health:   func(context.Context, string) error { return errors.New("unhealthy") }})
	if err := registry.ActivateAll(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	statuses := registry.CheckHealth(context.Background())
	if statuses[0].State != StateQuarantined {
		t.Fatalf("statuses=%+v", statuses)
	}
	registry = NewRegistry[string]()
	_ = registry.Register(Contribution[string]{ID: "a", Version: "1", Owner: "host", Value: "old", Activate: func(context.Context, string) error { restored = true; return nil }})
	_ = registry.ActivateAll(context.Background(), []string{"a"})
	restored = false
	err := registry.Update(context.Background(), Contribution[string]{ID: "a", Version: "2", Owner: "host", Value: "new", Activate: func(context.Context, string) error { return errors.New("broken update") }})
	if err == nil || !restored {
		t.Fatalf("update err=%v restored=%v", err, restored)
	}
}

func TestCrossRegistryTransactionRollsBackInReverse(t *testing.T) {
	var events []string
	err := ApplyTransaction(context.Background(), []Step{
		{Name: "tools", Apply: func(context.Context) error { events = append(events, "tools+"); return nil }, Rollback: func(context.Context) error { events = append(events, "tools-"); return nil }},
		{Name: "skills", Apply: func(context.Context) error { return errors.New("broken") }},
	})
	if err == nil || len(events) != 2 || events[1] != "tools-" {
		t.Fatalf("events=%v err=%v", events, err)
	}
}
