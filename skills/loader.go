package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillMeta is the YAML frontmatter shape for skill.md files.
type skillMeta struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Category      string `yaml:"category"`
	Priority      int    `yaml:"priority"`
	TokenEstimate int    `yaml:"token_estimate"`
	Scope         string `yaml:"scope"`
	Source        string `yaml:"source"`
	Trust         string `yaml:"trust"`
	SchemaVersion int    `yaml:"schema_version"`
	Precedence    int    `yaml:"precedence"`
}

// Diagnostic records a quarantined skill that was not activated.
type Diagnostic struct {
	Path  string
	Error string
}

// LoadOptions controls bounded, partial skill discovery.
type LoadOptions struct {
	ContinueOnError bool
	MaxSkillBytes   int
	MaxTotalBytes   int
	OnDiagnostic    func(Diagnostic)
}

// LoadReport summarizes one discovery pass.
type LoadReport struct {
	Loaded      []string
	Quarantined []Diagnostic
	TotalBytes  int
}

// LoadFS scans `root` inside fsys for `<slug>/SKILL.md` (or the
// lowercase `<slug>/skill.md`) entries and parses each into a Skill.
// Reference files live under `<slug>/references/*.md` and become
// entries in Skill.References keyed by their basename (without .md).
//
// `SKILL.md` is the casing used by Anthropic's public Agent Skills
// spec (anthropics/skills) and is the preferred name for new skills;
// `skill.md` is kept for backwards compatibility with older content.
//
// Missing references directories are not errors; malformed frontmatter is.
//
// Typical usage from a consumer:
//
//	//go:embed all:content
//	var skillFS embed.FS
//
//	reg := skills.NewRegistry()
//	if err := skills.LoadFS(reg, skillFS, "content"); err != nil { ... }
func LoadFS(reg *Registry, fsys fs.FS, root string) error {
	_, err := LoadFSWithOptions(reg, fsys, root, LoadOptions{})
	return err
}

// LoadFSWithOptions can quarantine malformed entries and continue. Strict
// LoadFS behavior remains the default when ContinueOnError is false.
func LoadFSWithOptions(reg *Registry, fsys fs.FS, root string, opts LoadOptions) (LoadReport, error) {
	var report LoadReport
	dirs, err := fs.ReadDir(fsys, root)
	if err != nil {
		return report, fmt.Errorf("read %s: %w", root, err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		// Prefer the Anthropic-standard SKILL.md, fall back to the
		// legacy lowercase variant. We try both literally rather than
		// doing a case-insensitive directory scan because some embed.FS
		// implementations and case-sensitive filesystems disagree on
		// what counts as a "match".
		var (
			data      []byte
			skillPath string
			readErr   error
		)
		for _, name := range [...]string{"SKILL.md", "skill.md"} {
			candidate := path.Join(root, d.Name(), name)
			data, readErr = fs.ReadFile(fsys, candidate)
			if readErr == nil {
				skillPath = candidate
				break
			}
		}
		if readErr != nil {
			// Subdir without a skill file is allowed (e.g. references-only).
			continue
		}
		if opts.MaxSkillBytes > 0 && len(data) > opts.MaxSkillBytes {
			if err := quarantineSkill(reg, &report, opts, skillPath, fmt.Errorf("skill exceeds %d bytes", opts.MaxSkillBytes)); err != nil {
				return report, err
			}
			continue
		}
		skill, err := parseSkillFile(d.Name(), data)
		if err != nil {
			if err := quarantineSkill(reg, &report, opts, skillPath, err); err != nil {
				return report, err
			}
			continue
		}
		refs, err := loadReferences(fsys, path.Join(root, d.Name(), "references"))
		if err != nil {
			if err := quarantineSkill(reg, &report, opts, skillPath, err); err != nil {
				return report, err
			}
			continue
		}
		total := len(data)
		for _, ref := range refs {
			total += len(ref)
		}
		if opts.MaxSkillBytes > 0 && total > opts.MaxSkillBytes {
			if err := quarantineSkill(reg, &report, opts, skillPath, fmt.Errorf("skill and references exceed %d bytes", opts.MaxSkillBytes)); err != nil {
				return report, err
			}
			continue
		}
		if opts.MaxTotalBytes > 0 && report.TotalBytes+total > opts.MaxTotalBytes {
			if err := quarantineSkill(reg, &report, opts, skillPath, fmt.Errorf("aggregate skill budget exceeds %d bytes", opts.MaxTotalBytes)); err != nil {
				return report, err
			}
			continue
		}
		skill.References = refs
		skill.Digest = digestSkill(skill)
		reg.Register(skill)
		report.Loaded = append(report.Loaded, skill.ID)
		report.TotalBytes += total
	}
	return report, nil
}

func quarantineSkill(reg *Registry, report *LoadReport, opts LoadOptions, skillPath string, cause error) error {
	diagnostic := Diagnostic{Path: skillPath, Error: cause.Error()}
	if !opts.ContinueOnError {
		return fmt.Errorf("parse %s: %w", skillPath, cause)
	}
	report.Quarantined = append(report.Quarantined, diagnostic)
	reg.addDiagnostic(diagnostic)
	if opts.OnDiagnostic != nil {
		opts.OnDiagnostic(diagnostic)
	}
	return nil
}

// ResolveWorkspacePath resolves a workspace-relative skills path without
// allowing absolute paths or traversal outside the admitted root.
func ResolveWorkspacePath(workspaceRoot, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("skill path must be workspace-relative")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(filepath.Join(root, relative))
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill path escapes workspace root")
	}
	return resolved, nil
}

// Watcher lets hosts connect filesystem or registry notifications without
// prescribing a platform-specific watch implementation.
type Watcher interface {
	Changes() <-chan struct{}
	Close() error
}

func loadReferences(fsys fs.FS, dir string) (map[string]string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	refs := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		refs[name] = string(data)
	}
	if len(refs) == 0 {
		return nil, nil
	}
	return refs, nil
}

// parseSkillFile splits the YAML frontmatter from the body and produces a
// Skill. dirName is used as a fallback ID if no id field is set.
func parseSkillFile(dirName string, data []byte) (*Skill, error) {
	raw := string(data)
	if !strings.HasPrefix(raw, "---\n") && !strings.HasPrefix(raw, "---\r\n") {
		return nil, fmt.Errorf("missing frontmatter delimiter (expected leading '---')")
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	rest := raw[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, fmt.Errorf("unterminated frontmatter (expected trailing '---')")
	}
	front := rest[:end]
	body := strings.TrimSpace(rest[end+5:])

	var meta skillMeta
	decoder := yaml.NewDecoder(strings.NewReader(front))
	decoder.KnownFields(true)
	if err := decoder.Decode(&meta); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	if meta.ID == "" {
		meta.ID = strings.ReplaceAll(dirName, "-", "_")
	}
	if meta.Name == "" {
		return nil, fmt.Errorf("skill %s: missing 'name' in frontmatter", meta.ID)
	}
	return &Skill{
		ID:            meta.ID,
		Name:          meta.Name,
		Description:   meta.Description,
		Category:      meta.Category,
		Scope:         meta.Scope,
		Source:        meta.Source,
		Trust:         meta.Trust,
		SchemaVersion: meta.SchemaVersion,
		Precedence:    meta.Precedence,
		Priority:      meta.Priority,
		TokenEstimate: meta.TokenEstimate,
		Content:       body,
	}, nil
}

func digestSkill(skill *Skill) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d\x00%s\x00%s\x00%s\x00%s\x00", skill.SchemaVersion, skill.ID, skill.Scope, skill.Trust, skill.Content)
	for _, name := range skill.ReferenceNames() {
		fmt.Fprintf(&buf, "%s\x00%s\x00", name, skill.References[name])
	}
	sum := sha256.Sum256(buf.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:])
}
