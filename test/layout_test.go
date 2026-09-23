package test

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	maxSourceLines = 700
	maxDirFiles    = 19
)

var sourceExtensions = map[string]struct{}{
	".go": {}, ".js": {}, ".jsx": {}, ".swift": {}, ".ts": {}, ".tsx": {},
}

func TestRepositoryLayout(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	production := make(map[string]int)
	tests := make(map[string]int)
	var violations []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == ".DS_Store" {
			violations = append(violations, fmt.Sprintf("metadata junk: %s", relative(root, path)))
			return nil
		}
		if _, supported := sourceExtensions[filepath.Ext(entry.Name())]; !supported {
			return nil
		}
		lines, err := lineCount(path)
		if err != nil {
			return err
		}
		if lines > maxSourceLines {
			violations = append(violations, fmt.Sprintf("%d lines: %s", lines, relative(root, path)))
		}
		dir := relative(root, filepath.Dir(path))
		if isTestSource(entry.Name()) {
			tests[dir]++
		} else {
			production[dir]++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	violations = append(violations, densityViolations("production", production)...)
	violations = append(violations, densityViolations("test", tests)...)
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("repository layout violations:\n%s", strings.Join(violations, "\n"))
	}
}

func isTestSource(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec.") || strings.HasSuffix(lower, "tests.swift")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate layout test")
	}
	return filepath.Dir(filepath.Dir(file))
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", ".dist", ".release", "bin", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func lineCount(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(content) == 0 {
		return 0, nil
	}
	lines := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		lines++
	}
	return lines, nil
}

func densityViolations(kind string, counts map[string]int) []string {
	var violations []string
	for dir, count := range counts {
		if count > maxDirFiles {
			violations = append(violations, fmt.Sprintf("%d %s source files: %s", count, kind, dir))
		}
	}
	return violations
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
