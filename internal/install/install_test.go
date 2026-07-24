package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danieljustus/symaira-skills/internal/render"
)

func TestInstallRefusesUnmanagedCollision(t *testing.T) {
	home := t.TempDir()
	rendered := t.TempDir()
	writeFile(t, filepath.Join(rendered, "SKILL.md"), "---\nname: collide\ndescription: test\n---\n")
	dest := filepath.Join(home, ".config", "opencode", "skills", "collide")
	writeFile(t, filepath.Join(dest, "SKILL.md"), "unmanaged")

	_, err := Install(RenderedSkill{
		Target: render.TargetOpenCode,
		Name:   "collide",
		Path:   rendered,
	}, Options{HomeDir: home, Scope: ScopeUser, Mode: ModeCopy})
	if err == nil {
		t.Fatal("expected unmanaged collision error")
	}
}

func TestInstallCopyWritesMarkerAndUninstallRemovesManagedSkill(t *testing.T) {
	home := t.TempDir()
	rendered := t.TempDir()
	writeFile(t, filepath.Join(rendered, "SKILL.md"), "---\nname: managed\ndescription: test\n---\n")

	result, err := Install(RenderedSkill{
		Target: render.TargetClaude,
		Name:   "managed",
		Path:   rendered,
	}, Options{HomeDir: home, Scope: ScopeUser, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if result.Action != "installed" {
		t.Fatalf("action: want installed, got %q", result.Action)
	}
	if _, err := os.Stat(filepath.Join(result.Path, ".symskills.json")); err != nil {
		t.Fatalf("marker missing: %v", err)
	}

	removed, err := Uninstall(render.TargetClaude, "managed", Options{HomeDir: home, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !removed {
		t.Fatal("expected Uninstall to report removed=true")
	}
	if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
		t.Fatalf("expected installed skill removed, stat err=%v", err)
	}
}

func TestInstallAndUninstallDanglingSymlink(t *testing.T) {
	home := t.TempDir()
	rendered := t.TempDir()
	writeFile(t, filepath.Join(rendered, "SKILL.md"), "---\nname: dangling\ndescription: test\n---\n")

	result, err := Install(RenderedSkill{
		Target: render.TargetClaude,
		Name:   "dangling",
		Path:   rendered,
	}, Options{HomeDir: home, Scope: ScopeUser, Mode: ModeSymlink})
	if err != nil {
		t.Fatalf("Install symlink: %v", err)
	}

	// Remove rendered source to make result.Path a dangling symlink
	if err := os.RemoveAll(rendered); err != nil {
		t.Fatal(err)
	}

	// Verify it is a dangling symlink
	if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
		t.Fatalf("expected stat to fail for dangling symlink")
	}

	// Re-install should succeed over dangling symlink
	newRendered := t.TempDir()
	writeFile(t, filepath.Join(newRendered, "SKILL.md"), "---\nname: dangling\ndescription: test\n---\n")
	_, err = Install(RenderedSkill{
		Target: render.TargetClaude,
		Name:   "dangling",
		Path:   newRendered,
	}, Options{HomeDir: home, Scope: ScopeUser, Mode: ModeSymlink})
	if err != nil {
		t.Fatalf("Re-install over dangling symlink failed: %v", err)
	}

	// Make dangling again for Uninstall test
	if err := os.RemoveAll(newRendered); err != nil {
		t.Fatal(err)
	}
	removed, err := Uninstall(render.TargetClaude, "dangling", Options{HomeDir: home, Scope: ScopeUser})
	if err != nil {
		t.Fatalf("Uninstall dangling symlink failed: %v", err)
	}
	if !removed {
		t.Fatal("expected Uninstall to report removed=true for dangling symlink")
	}
	if _, err := os.Lstat(result.Path); !os.IsNotExist(err) {
		t.Fatalf("expected dangling symlink removed")
	}
}

func TestDiffReportsChangedFiles(t *testing.T) {
	rendered := t.TempDir()
	installed := t.TempDir()
	writeFile(t, filepath.Join(rendered, "SKILL.md"), "new")
	writeFile(t, filepath.Join(installed, "SKILL.md"), "old")

	changes, err := Diff(rendered, installed)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "SKILL.md" || changes[0].Status != "modified" {
		t.Fatalf("unexpected changes: %#v", changes)
	}
}

func TestInstallPathRejectsHostileNames(t *testing.T) {
	hostile := []string{"../evil", "evil/name", "/etc/evil", ".."}
	for _, name := range hostile {
		_, err := InstallPath(render.TargetOpenCode, name, Options{})
		if err == nil {
			t.Fatalf("expected error for name %q", name)
		}
	}
}

func TestInstallPathScopeProject(t *testing.T) {
	project := t.TempDir()
	opts := Options{ProjectDir: project, Scope: ScopeProject}

	cases := []struct {
		target render.Target
		sub    []string
	}{
		{render.TargetOpenCode, []string{".opencode", "skills", "my-skill"}},
		{render.TargetClaude, []string{".claude", "skills", "my-skill"}},
		{render.TargetCodex, []string{".agents", "skills", "my-skill"}},
		{render.TargetHermes, []string{".hermes", "skills", "my-skill"}},
	}
	for _, c := range cases {
		got, err := InstallPath(c.target, "my-skill", opts)
		if err != nil {
			t.Fatalf("InstallPath(%s, ScopeProject): %v", c.target, err)
		}
		want := filepath.Join(append([]string{project}, c.sub...)...)
		if got != want {
			t.Errorf("InstallPath(%s, ScopeProject) = %q, want %q", c.target, got, want)
		}
	}

	// unknown target with project scope
	_, err := InstallPath("unknown-target", "my-skill", opts)
	if err == nil {
		t.Fatal("expected error for unknown target with project scope")
	}
}

func TestInstallPathUserAllTargets(t *testing.T) {
	home := t.TempDir()
	opts := Options{HomeDir: home, Scope: ScopeUser}

	cases := []struct {
		target render.Target
		sub    []string
	}{
		{render.TargetOpenCode, []string{".config", "opencode", "skills", "my-skill"}},
		{render.TargetClaude, []string{".claude", "skills", "my-skill"}},
		{render.TargetCodex, []string{".agents", "skills", "my-skill"}},
		{render.TargetHermes, []string{".hermes", "skills", "symaira", "my-skill"}},
	}
	for _, c := range cases {
		got, err := InstallPath(c.target, "my-skill", opts)
		if err != nil {
			t.Fatalf("InstallPath(%s, ScopeUser): %v", c.target, err)
		}
		want := filepath.Join(append([]string{home}, c.sub...)...)
		if got != want {
			t.Errorf("InstallPath(%s, ScopeUser) = %q, want %q", c.target, got, want)
		}
	}

	// unknown target with user scope
	_, err := InstallPath("unknown-target", "my-skill", opts)
	if err == nil {
		t.Fatal("expected error for unknown target with user scope")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
