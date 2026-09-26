package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Anurag607/amoeba/moe"
)

func TestClientDiscoversVersionAndModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"tiny:3b","size":123,"details":{"parameter_size":"3B"}},{"name":"large:32b","details":{"parameter_size":"32B"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	version, err := client.Version(context.Background())
	if err != nil || version != "1.2.3" {
		t.Fatalf("Version() = %q, %v", version, err)
	}
	candidates, err := client.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].Tier != moe.ModelTierFast || candidates[1].Tier != moe.ModelTierStrong {
		t.Fatalf("Candidates() = %+v", candidates)
	}
}

func TestClientRejectsRemoteByDefault(t *testing.T) {
	if _, err := New(Config{BaseURL: "https://example.com"}); err == nil {
		t.Fatal("New() error = nil, want remote rejection")
	}
}

func TestClientBoundsResponsesAndRedactsErrors(t *testing.T) {
	secret := "do-not-echo-this-token"
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		maxBody int64
		want    string
	}{
		{"oversized", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"version":"`+strings.Repeat("x", 64)+`"}`)
		}, 16, "exceeds"},
		{"invalid json", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{`) }, 1024, "decode"},
		{"empty version", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"version":" "}`) }, 1024, "omitted"},
		{"status body", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, secret, http.StatusInternalServerError) }, 1024, "500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()
			client, err := New(Config{BaseURL: server.URL, MaxBodyBytes: tc.maxBody})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Version(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), secret) {
				t.Fatalf("Version() error = %v", err)
			}
		})
	}
}

func TestClientCapsCustomTimeoutAndRejectsRedirect(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, `{"version":"1"}`)
	}))
	defer slow.Close()
	client, err := New(Config{BaseURL: slow.URL, Timeout: 25 * time.Millisecond, HTTPClient: &http.Client{Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Version(context.Background())
	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline")) {
		t.Fatalf("Version() error = %v, want deadline", err)
	}

	var followed bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed = true
		_, _ = io.WriteString(w, `{"version":"unsafe"}`)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	client, err = New(Config{BaseURL: redirect.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Version(context.Background()); err == nil || followed {
		t.Fatalf("Version() error = %v, followed = %t", err, followed)
	}
}

func TestDiscoverUsesBasePathAndNormalizesModels(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ollama/api/version":
			_, _ = io.WriteString(w, `{"version":"1"}`)
		case "/ollama/api/tags":
			_, _ = io.WriteString(w, `{"models":[{"name":" "},{"name":"model:7b","size":-10}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL + "/ollama/"})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := client.Discover(context.Background())
	if err != nil || len(candidates) != 1 || candidates[0].Resources.ResidentMemoryBytes != 0 {
		t.Fatalf("Discover() = %+v, %v", candidates, err)
	}
	if strings.Join(paths, ",") != "/ollama/api/version,/ollama/api/tags" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestTierForParameterSize(t *testing.T) {
	for _, tc := range []struct {
		size string
		want moe.ModelTier
	}{{"800M", moe.ModelTierFast}, {"7B", moe.ModelTierBalanced}, {"70B", moe.ModelTierStrong}} {
		var value model
		value.Details.ParameterSize = tc.size
		if got := tierFor(value); got != tc.want {
			t.Errorf("tierFor(%q) = %s, want %s", tc.size, got, tc.want)
		}
	}
}
