package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Anurag607/amoeba/trajectory"
)

type mockHarness struct{ configured bool }

func (*mockHarness) Name() string                        { return "named" }
func (*mockHarness) Validate(config HarnessConfig) error { return nil }
func (h *mockHarness) Configure(context.Context, HarnessConfig) error {
	h.configured = true
	return nil
}
func (*mockHarness) Version(context.Context) (string, error) { return "1", nil }
func (*mockHarness) Run(context.Context, HarnessRequest, func(trajectory.Event) error) (HarnessResult, error) {
	return HarnessResult{Status: "complete"}, nil
}
func (*mockHarness) Resume(context.Context, HarnessRequest, func(trajectory.Event) error) (HarnessResult, error) {
	return HarnessResult{Status: "complete"}, nil
}
func (*mockHarness) Cancel(context.Context, string) error { return nil }
func (h *mockHarness) Status(context.Context) (HarnessStatus, error) {
	return HarnessStatus{Name: h.Name(), Configured: h.configured, Health: HealthReady}, nil
}
func (*mockHarness) Health(context.Context) (Health, error) { return HealthReady, nil }
func (*mockHarness) Capabilities(context.Context) (HarnessCapabilities, error) {
	return HarnessCapabilities{ResumeToken: true, TranscriptReplay: true, ExactCheckpoint: true, EventReplay: true, IdempotentResume: true}, nil
}

func TestHarnessRegistryRequiresNamedAllowlistAndConfiguration(t *testing.T) {
	registry := NewHarnessRegistry("named")
	harness := &mockHarness{}
	if err := registry.Register(harness); err != nil {
		t.Fatal(err)
	}
	if err := registry.Configure(context.Background(), "named", HarnessConfig{Name: "named", Executable: "/bin/named"}); err != nil {
		t.Fatal(err)
	}
	statuses, err := registry.Statuses(context.Background())
	if err != nil || len(statuses) != 1 || !statuses[0].Configured {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
	manifest, err := registry.Manifest(context.Background(), "named")
	if err != nil || manifest.ConfigDigest == "" || manifest.Version != "1" || !manifest.Capabilities.ExactCheckpoint {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	if err := NewHarnessRegistry().Register(&mockHarness{}); err == nil {
		t.Fatal("unallowlisted harness registered")
	}
}

type mockMCP struct{ stopped, authenticated bool }

func (*mockMCP) Validate(MCPConfig) error                     { return nil }
func (*mockMCP) Configure(context.Context, MCPConfig) error   { return nil }
func (*mockMCP) Start(context.Context) error                  { return nil }
func (m *mockMCP) Authenticate(context.Context) error         { m.authenticated = true; return nil }
func (*mockMCP) RefreshAuth(context.Context) error            { return nil }
func (*mockMCP) Discover(context.Context) (MCPCatalog, error) { return MCPCatalog{Version: "v1"}, nil }
func (*mockMCP) WatchCatalog(context.Context) (<-chan MCPCatalog, error) {
	return make(chan MCPCatalog), nil
}
func (*mockMCP) CallTool(context.Context, MCPToolCall) (MCPToolResult, error) {
	return MCPToolResult{}, nil
}
func (*mockMCP) ReadResource(context.Context, MCPResourceRequest) (json.RawMessage, error) {
	return nil, nil
}
func (*mockMCP) RenderPrompt(context.Context, MCPPromptRequest) (json.RawMessage, error) {
	return nil, nil
}
func (*mockMCP) Health(context.Context) (Health, error) { return HealthReady, nil }
func (m *mockMCP) Status(context.Context) (MCPStatus, error) {
	return MCPStatus{Health: HealthReady, Authenticated: m.authenticated}, nil
}
func (m *mockMCP) Stop(context.Context) error { m.stopped = true; return nil }

func TestMCPRegistryOwnsAuthCatalogAndCleanupLifecycle(t *testing.T) {
	registry := NewMCPRegistry("server")
	server := &mockMCP{}
	config := MCPConfig{Name: "server", Version: "1", Transport: TransportHTTP, Endpoint: "https://example.invalid", Auth: AuthOAuth}
	if err := registry.Register(context.Background(), config, server); err != nil {
		t.Fatal(err)
	}
	if catalog, ok := registry.Catalog("server"); !ok || catalog.Version != "v1" || !server.authenticated {
		t.Fatalf("catalog=%+v authenticated=%v", catalog, server.authenticated)
	}
	if err := registry.Unregister(context.Background(), "server"); err != nil || !server.stopped {
		t.Fatalf("stopped=%v err=%v", server.stopped, err)
	}
}
