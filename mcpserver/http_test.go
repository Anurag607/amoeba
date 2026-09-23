package mcpserver

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anurgosw/agentic-moe/runtimekit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStreamableHTTPConformance(t *testing.T) {
	server := newHTTPTestServer(t)
	handler, err := server.HTTPHandler(testHTTPOptions())
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "http-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 3 {
		t.Fatalf("ListTools() = %+v, %v", tools, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "plan_task", Arguments: map[string]any{"query": "review this code"},
	})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("CallTool() = %+v, %v", result, err)
	}
}

func TestHTTPHandlerSecurityAndLimits(t *testing.T) {
	server := newHTTPTestServer(t)
	opts := testHTTPOptions()
	opts.AllowedOrigins = []string{"https://trusted.example"}
	handler, err := server.HTTPHandler(opts)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(`{}`))
		req.Header.Set("Origin", "https://blocked.example")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("security headers = %v", recorder.Header())
		}
	})

	t.Run("body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", bytes.NewReader(bytes.Repeat([]byte("x"), int(opts.MaxBodyBytes+1))))
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
		}
	})
}

func TestHTTPHandlerBoundsConcurrencyAndDeadline(t *testing.T) {
	server := newHTTPTestServer(t)
	entered := make(chan struct{})
	mcp.AddTool(server.ProtocolServer(), &mcp.Tool{Name: "block"},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			close(entered)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		})
	opts := testHTTPOptions()
	opts.RequestTimeout = 150 * time.Millisecond
	handler, err := server.HTTPHandler(opts)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "load-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "block", Arguments: map[string]any{}})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("blocking tool was not entered")
	}

	req, err := http.NewRequest(http.MethodPost, httpServer.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusTooManyRequests)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not stop at its deadline")
	}
}

func TestValidateHTTPOptionsRejectsUnsafeValues(t *testing.T) {
	for name, mutate := range map[string]func(*HTTPOptions){
		"wildcard without hosts": func(o *HTTPOptions) { o.Address = "0.0.0.0:8080"; o.BearerToken = "token" },
		"remote without token":   func(o *HTTPOptions) { o.Address = "192.0.2.1:8080" },
		"host with port":         func(o *HTTPOptions) { o.AllowedHosts = []string{"localhost:8080"} },
		"origin credentials":     func(o *HTTPOptions) { o.AllowedOrigins = []string{"https://user@example.com"} },
		"origin path":            func(o *HTTPOptions) { o.AllowedOrigins = []string{"https://example.com/path"} },
		"token whitespace":       func(o *HTTPOptions) { o.BearerToken = "bad token" },
		"zero timeout":           func(o *HTTPOptions) { o.RequestTimeout = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			opts := testHTTPOptions()
			mutate(&opts)
			if err := validateHTTPOptions(opts); err == nil {
				t.Fatal("validateHTTPOptions() error = nil")
			}
		})
	}
}

func TestRunHTTPStopsOnCancellation(t *testing.T) {
	server := newHTTPTestServer(t)
	opts := testHTTPOptions()
	opts.Address = "127.0.0.1:0"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- server.RunHTTP(ctx, opts) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunHTTP did not stop after cancellation")
	}
}

func FuzzHTTPValidators(f *testing.F) {
	f.Add("localhost:8080", "127.0.0.1:8080", "localhost", "Bearer token", "token")
	f.Add("[::1]:8080", "[::1]:8080", "::1", "bearer token", "token")
	f.Fuzz(func(t *testing.T, requestHost, address, allowed, header, token string) {
		_ = validHost(requestHost, address, []string{allowed})
		_ = validBearer(header, token)
	})
}

func newHTTPTestServer(t *testing.T) *Server {
	t.Helper()
	runtime, err := runtimekit.New(runtimekit.DefaultConfig(), runtimekit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func testHTTPOptions() HTTPOptions {
	return HTTPOptions{Address: "127.0.0.1:8080", MaxBodyBytes: 1024, MaxConcurrent: 1, RequestTimeout: time.Second, ShutdownGrace: time.Second}
}
