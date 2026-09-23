package configgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTargets(t *testing.T) {
	for _, target := range []string{"generic", "codex", "vscode", "ollama"} {
		body, path, err := Render(target, "agentic-moe", "config.yaml")
		if err != nil {
			t.Fatalf("Render(%q) error = %v", target, err)
		}
		if len(body) == 0 || path == "" {
			t.Fatalf("Render(%q) returned empty output", target)
		}
	}
}

func TestRenderProducesExactHostShapes(t *testing.T) {
	body, path, err := Render("generic", "/opt/agentic-moe", "/tmp/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if path != ".mcp.json" {
		t.Fatalf("path = %q", path)
	}
	var generic struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatal(err)
	}
	server := generic.Servers["agentic-moe"]
	if server.Command != "/opt/agentic-moe" || strings.Join(server.Args, " ") != "mcp stdio --config /tmp/config.yaml" {
		t.Fatalf("generic server = %+v", server)
	}

	body, path, err = Render("codex", "agentic-moe", "")
	if err != nil || path != "agentic-moe.codex.toml" || !strings.Contains(string(body), "[mcp_servers.agentic-moe]") {
		t.Fatalf("codex = %q, %q, %v", body, path, err)
	}
}

func TestRenderRejectsUnknownTarget(t *testing.T) {
	if _, _, err := Render("unknown", "", ""); err == nil {
		t.Fatal("Render() error = nil")
	}
}

func TestWriteRefusesExistingAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Write(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Write() error = %v", err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Write(link, []byte("unsafe"), true); err == nil {
		t.Fatal("Write() error = nil, want symlink rejection")
	}
}

func TestWriteForceReplacesRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	if err := Write(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), true); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "second" {
		t.Fatalf("ReadFile() = %q, %v", body, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o022 != 0 {
		t.Fatalf("mode = %v", info.Mode())
	}
}
