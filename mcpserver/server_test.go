package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anurgosw/agentic-moe/runtimekit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInMemoryToolsAndResources(t *testing.T) {
	ctx := context.Background()
	runtime, err := runtimekit.New(runtimekit.DefaultConfig(), runtimekit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.ProtocolServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 3 {
		t.Fatalf("ListTools() = %+v, %v", tools, err)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "plan_task", Arguments: map[string]any{"query": "debug code"}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("CallTool() = %+v, %v", result, err)
	}
	invalid, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "validate_config", Arguments: map[string]any{"config": "version: 99"}})
	if err != nil || invalid.IsError {
		t.Fatalf("validate_config = %+v, %v", invalid, err)
	}
	validation, ok := invalid.StructuredContent.(map[string]any)
	if !ok || validation["valid"] != false || validation["error"] != "configuration is invalid" {
		t.Fatalf("validation = %#v", invalid.StructuredContent)
	}
	resource, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "agentic-moe://manifest"})
	if err != nil || len(resource.Contents) != 1 || resource.Contents[0].Text == "" {
		t.Fatalf("ReadResource() = %+v, %v", resource, err)
	}
}

func TestHTTPHandlerRequiresRemoteAuthenticationAndChecksBearer(t *testing.T) {
	runtime, err := runtimekit.New(runtimekit.DefaultConfig(), runtimekit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	base := HTTPOptions{Address: "0.0.0.0:8080", AllowedHosts: []string{"localhost"}, MaxBodyBytes: 1024, MaxConcurrent: 1, RequestTimeout: 1000000000, ShutdownGrace: 1000000000}
	if _, err := server.HTTPHandler(base); err == nil {
		t.Fatal("HTTPHandler() error = nil, want remote auth requirement")
	}
	base.BearerToken = "secret-value"
	handler, err := server.HTTPHandler(base)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "http://localhost/mcp", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://blocked.example/mcp", nil)
	request.Header.Set("Authorization", "Bearer secret-value")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("blocked host status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}
