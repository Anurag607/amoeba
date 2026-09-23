package configgen

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes generated content, refusing existing paths unless force is set.
func Write(path string, body []byte, force bool) error {
	if path == "" {
		return fmt.Errorf("config generator: output path is required")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("config generator: refusing symlink output %q", path)
		}
		if !force {
			return fmt.Errorf("config generator: %q already exists", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("config generator: inspect output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config generator: create parent: %w", err)
	}
	if force {
		return replace(path, body)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("config generator: create output: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return fmt.Errorf("config generator: write output: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("config generator: close output: %w", err)
	}
	return nil
}

func replace(path string, body []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".agentic-moe-config-*")
	if err != nil {
		return fmt.Errorf("config generator: create temporary output: %w", err)
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return fmt.Errorf("config generator: set output mode: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return fmt.Errorf("config generator: write output: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("config generator: close output: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("config generator: replace output: %w", err)
	}
	return nil
}
