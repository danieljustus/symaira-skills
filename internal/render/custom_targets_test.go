package render

import (
	"testing"
)

// customTargetsSnapshot returns the length of the global registry so tests
// can assert on the delta without depending on the built-in count.
func customTargetsSnapshot() int {
	return len(Targets)
}

func TestRegisterCustomTargetsAppendsToRegistry(t *testing.T) {
	before := customTargetsSnapshot()
	defer func() { Targets = Targets[:before] }()

	err := RegisterCustomTargets([]CustomTargetSpec{
		{Name: "myagent", SkillRootUser: "/home/u/.myagent/skills"},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(Targets) != before+1 {
		t.Fatalf("expected %d targets, got %d", before+1, len(Targets))
	}
	spec, ok := LookupSpec("myagent")
	if !ok {
		t.Fatal("custom target not found in registry")
	}
	if got := spec.SkillRoot("/home/u", "/proj", ScopeUser); got != "/home/u/.myagent/skills" {
		t.Errorf("user skill root = %q", got)
	}
	if got := spec.SkillRoot("/home/u", "/proj", ScopeProject); got != "/home/u/.myagent/skills" {
		t.Errorf("project skill root defaults to user root, got %q", got)
	}
	if got := spec.ConfigDir("/home/u", "/proj", ScopeUser); got != "/home/u/.myagent" {
		t.Errorf("config dir = %q", got)
	}
	// ParseTarget must accept the custom name.
	if _, err := ParseTarget("myagent"); err != nil {
		t.Errorf("ParseTarget(custom): %v", err)
	}
}

func TestRegisterCustomTargetsCollision(t *testing.T) {
	before := customTargetsSnapshot()
	defer func() { Targets = Targets[:before] }()

	err := RegisterCustomTargets([]CustomTargetSpec{
		{Name: "opencode", SkillRootUser: "/tmp/x"},
	})
	if err == nil {
		t.Fatal("expected collision error for built-in name opencode")
	}
	if len(Targets) != before {
		t.Errorf("registry mutated on collision: %d -> %d", before, len(Targets))
	}

	// Duplicate custom names must also collide.
	if err := RegisterCustomTargets([]CustomTargetSpec{{Name: "myagent", SkillRootUser: "/tmp/a"}}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := RegisterCustomTargets([]CustomTargetSpec{{Name: "myagent", SkillRootUser: "/tmp/b"}}); err == nil {
		t.Fatal("expected collision error for duplicate custom name")
	}
}

func TestRegisterCustomTargetsValidation(t *testing.T) {
	before := customTargetsSnapshot()
	defer func() { Targets = Targets[:before] }()

	if err := RegisterCustomTargets([]CustomTargetSpec{{Name: "", SkillRootUser: "/tmp/x"}}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := RegisterCustomTargets([]CustomTargetSpec{{Name: "x", SkillRootUser: ""}}); err == nil {
		t.Fatal("expected error for empty skill_root_user")
	}
}

func TestCustomTargetOverlayDir(t *testing.T) {
	before := customTargetsSnapshot()
	defer func() { Targets = Targets[:before] }()

	if err := RegisterCustomTargets([]CustomTargetSpec{
		{Name: "custom", SkillRootUser: "/tmp/custom-root", OverlayDir: "my-overlays"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := overlayDir("custom"); got != "my-overlays" {
		t.Errorf("overlayDir = %q, want my-overlays", got)
	}
	if got := overlayDir("opencode"); got != "opencode" {
		t.Errorf("built-in overlayDir = %q, want opencode", got)
	}
}
