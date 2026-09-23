package skills

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/anurgosw/agentic-moe/policy"
)

func TestRegistryOrderingAndBudgetAreDeterministic(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Skill{ID: "z_last", Name: "Z", Priority: 2, Content: strings.Repeat("z", 80), TokenEstimate: 20})
	reg.Register(&Skill{ID: "b_second", Name: "B", Priority: 1, Content: strings.Repeat("b", 80), TokenEstimate: 20})
	reg.Register(&Skill{ID: "a_first", Name: "A", Priority: 1, Content: strings.Repeat("a", 80), TokenEstimate: 20})
	if got, want := reg.AllIDs(), []string{"a_first", "b_second", "z_last"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs not stable: got %v want %v", got, want)
	}
	context, loaded, omitted := reg.AssembleContextBudget([]string{"z_last", "b_second", "a_first"}, 80)
	if !reflect.DeepEqual(loaded, []string{"a_first", "b_second"}) || !reflect.DeepEqual(omitted, []string{"z_last"}) {
		t.Fatalf("unexpected budget selection loaded=%v omitted=%v", loaded, omitted)
	}
	if strings.Index(context, "SKILL: A") > strings.Index(context, "SKILL: B") {
		t.Fatal("equal-priority skills were not ordered by ID")
	}
}

func TestScopePrecedencePolicyVisibilityAndWorkspacePath(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&Skill{ID: "same", Name: "Builtin", Scope: "builtin"})
	registry.Register(&Skill{ID: "same", Name: "Project", Scope: "project"})
	registry.Register(&Skill{ID: "hidden", Name: "Hidden", Scope: "project"})
	skill, _ := registry.Get("same")
	if skill.Name != "Project" {
		t.Fatalf("scope precedence failed: %+v", skill)
	}
	snapshot, _ := policy.NewSnapshot("p1", policy.RuleSet{Layer: policy.LayerGlobal, Rules: []policy.Rule{
		{Action: "skill.same", Effect: policy.EffectAllow, ReasonCode: "visible"},
		{Action: "skill.hidden", Effect: policy.EffectDeny, ReasonCode: "hidden"},
	}})
	if visible := registry.Visible(snapshot); len(visible) != 1 || visible[0].ID != "same" {
		t.Fatalf("visible=%v", visible)
	}
	root := t.TempDir()
	resolved, err := ResolveWorkspacePath(root, filepath.Join(".agents", "skills"))
	if err != nil || resolved == "" {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	if _, err := ResolveWorkspacePath(root, filepath.Join("..", "escape")); err == nil {
		t.Fatal("workspace escape accepted")
	}
}

func TestLoadFSWithOptionsQuarantinesMalformedSkills(t *testing.T) {
	files := fstest.MapFS{
		"content/good/SKILL.md": {Data: []byte("---\nname: Good\nschema_version: 1\nscope: project\ntrust: host\n---\nUseful content")},
		"content/bad/SKILL.md":  {Data: []byte("not frontmatter")},
	}
	registry := NewRegistry()
	report, err := LoadFSWithOptions(registry, files, "content", LoadOptions{ContinueOnError: true, MaxTotalBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.Loaded, []string{"good"}) || len(report.Quarantined) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	skill, ok := registry.Get("good")
	if !ok || !strings.HasPrefix(skill.Digest, "sha256:") || skill.SchemaVersion != 1 {
		t.Fatalf("missing skill metadata: %+v", skill)
	}
}

func TestRegistryHonorsSkillPrecedence(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&Skill{ID: "same", Name: "Project", Precedence: 20})
	registry.Register(&Skill{ID: "same", Name: "Builtin", Precedence: 10})
	skill, _ := registry.Get("same")
	if skill.Name != "Project" {
		t.Fatalf("lower-precedence skill replaced active skill: %+v", skill)
	}
}
