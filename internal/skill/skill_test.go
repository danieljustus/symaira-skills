package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBundleParsesFrontmatterAndManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), `---
name: sample-skill
description: Use when testing Symaira skill parsing.
license: Apache-2.0
metadata:
  audience: maintainers
---

# Sample Skill

Follow the workflow.
`)
	writeFile(t, filepath.Join(dir, "symskills.toml"), `[skill]
name = "sample-skill"
version = "1.2.3"
source = "https://example.test/sample-skill"

[targets.opencode]
enabled = true
alias = "sample-opencode"

[targets.claude]
enabled = false
`)

	bundle, err := LoadBundle(dir)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}

	if bundle.Frontmatter.Name != "sample-skill" {
		t.Fatalf("name: want sample-skill, got %q", bundle.Frontmatter.Name)
	}
	if bundle.Frontmatter.Metadata["audience"] != "maintainers" {
		t.Fatalf("metadata audience not parsed: %#v", bundle.Frontmatter.Metadata)
	}
	if bundle.Manifest.Skill.Version != "1.2.3" {
		t.Fatalf("manifest version: want 1.2.3, got %q", bundle.Manifest.Skill.Version)
	}
	if !bundle.Manifest.Targets["opencode"].Enabled {
		t.Fatal("opencode target should be enabled")
	}
	if bundle.Manifest.Targets["opencode"].Alias != "sample-opencode" {
		t.Fatal("opencode alias should be parsed")
	}
	if bundle.Manifest.Targets["claude"].Enabled {
		t.Fatal("claude target should be disabled")
	}
	if bundle.Body != "# Sample Skill\n\nFollow the workflow.\n" {
		t.Fatalf("body mismatch: %q", bundle.Body)
	}
}

func TestValidateRejectsInvalidNamesAndMissingDescription(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), `---
name: Bad_Name
description: ""
---

Body.
`)

	bundle, err := LoadBundle(dir)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}

	issues := Validate(bundle)
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
	if !HasIssue(issues, "name_format") {
		t.Fatalf("expected name_format issue, got %#v", issues)
	}
	if !HasIssue(issues, "description_required") {
		t.Fatalf("expected description_required issue, got %#v", issues)
	}
}

func TestValidateSkillNameAcceptsValidAndRejectsInvalidNames(t *testing.T) {
	valid := []string{"repo-review", "a", "my-skill-1", "opencode-123"}
	for _, name := range valid {
		if err := ValidateSkillName(name); err != nil {
			t.Fatalf("expected %q to be valid, got %v", name, err)
		}
	}
	invalid := []string{"", "Bad_Name", "evil/name", "../evil", "/etc/evil", "..", "skill.name", "UPPER"}
	for _, name := range invalid {
		if err := ValidateSkillName(name); err == nil {
			t.Fatalf("expected %q to be invalid", name)
		}
	}
}

func TestImportSkillCopiesExistingSkillIntoLibrary(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "SKILL.md"), `---
name: import-me
description: Import this existing OpenCode style skill.
---

Body.
`)
	writeFile(t, filepath.Join(src, "references", "details.md"), "Details\n")

	dst := filepath.Join(t.TempDir(), "library")
	imported, err := ImportSkill(src, dst)
	if err != nil {
		t.Fatalf("ImportSkill: %v", err)
	}

	if imported.Name != "import-me" {
		t.Fatalf("imported name: %q", imported.Name)
	}
	if _, err := os.Stat(filepath.Join(dst, "import-me", "SKILL.md")); err != nil {
		t.Fatalf("imported SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "import-me", "references", "details.md")); err != nil {
		t.Fatalf("imported reference missing: %v", err)
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
