package skill

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-skills/internal/variant"
)

func loadForValidation(t *testing.T, root string) *Bundle {
	t.Helper()
	bundle, err := LoadBundle(root)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	return bundle
}

func issueFor(issues []Issue, code string) (Issue, bool) {
	for _, issue := range issues {
		if issue.Code == code {
			return issue, true
		}
	}
	return Issue{}, false
}

// withKnownTargets installs a target registry for the duration of one test.
// Production wiring comes from internal/render's init; the skill package must
// not depend on it, so tests set the hook explicitly.
func withKnownTargets(t *testing.T, names ...string) {
	t.Helper()
	previous := KnownTargets
	KnownTargets = func() []string { return names }
	t.Cleanup(func() { KnownTargets = previous })
}

func TestValidateAcceptsWellFormedVariants(t *testing.T) {
	withKnownTargets(t, "claude", "hermes")
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill with harness-specific blocks and terms.
category: developer-tools
---

<!-- symskills:block worker -->
Canonical worker text.
<!-- /symskills:block -->

Reports go to {{term:report_dir}}.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[terms.report_dir]
default = "~/.local/state/symskills/reports"
hermes = "~/.hermes/reports"
`)
	writeFile(t, filepath.Join(root, "overlays", "claude", "blocks", "worker.md"), "Claude worker text.\n")

	for _, issue := range Validate(loadForValidation(t, root)) {
		if issue.Severity == "error" {
			t.Errorf("unexpected error: %s — %s (%s)", issue.Code, issue.Message, issue.Path)
		}
	}
}

func TestValidateRejectsOverrideForUnknownBlock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill whose overlay overrides a block it does not define.
---

Body without any block.
`)
	writeFile(t, filepath.Join(root, "overlays", "claude", "blocks", "invented.md"), "text\n")

	issue, ok := issueFor(Validate(loadForValidation(t, root)), variant.CodeOverrideUnknown)
	if !ok {
		t.Fatalf("expected %s", variant.CodeOverrideUnknown)
	}
	if issue.Severity != "error" || !IsRenderBlocking(issue.Code) {
		t.Errorf("an override with no matching block must block the render: %+v", issue)
	}
	if !strings.Contains(issue.Message, "invented") {
		t.Errorf("message must name the block: %s", issue.Message)
	}
}

// TestValidateResolvesBlocksInReferences covers the case the real issue-sweep
// skill has: the harness-bound contract lives in a markdown reference, not in
// SKILL.md, so an override there must validate against it.
func TestValidateResolvesBlocksInReferences(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill whose overridable block lives in a reference file.
---

See references/execution-contract.md.
`)
	writeFile(t, filepath.Join(root, "references", "execution-contract.md"),
		"<!-- symskills:block dispatch -->\nCanonical dispatch.\n<!-- /symskills:block -->\n")
	writeFile(t, filepath.Join(root, "overlays", "claude", "blocks", "dispatch.md"), "Claude dispatch.\n")

	if _, ok := issueFor(Validate(loadForValidation(t, root)), variant.CodeOverrideUnknown); ok {
		t.Error("a block defined in a markdown reference must satisfy an override")
	}
}

func TestValidateRejectsDuplicateBlockIDAcrossFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill that reuses one block id in two files.
---

<!-- symskills:block dispatch -->
One.
<!-- /symskills:block -->
`)
	writeFile(t, filepath.Join(root, "references", "other.md"),
		"<!-- symskills:block dispatch -->\nTwo.\n<!-- /symskills:block -->\n")

	issue, ok := issueFor(Validate(loadForValidation(t, root)), variant.CodeBlockDuplicateID)
	if !ok {
		t.Fatalf("expected %s: ids address a block across the whole skill", variant.CodeBlockDuplicateID)
	}
	if !strings.Contains(issue.Message, "SKILL.md") {
		t.Errorf("message must name the first definition: %s", issue.Message)
	}
}

func TestValidateRejectsUnknownAndDefaultlessTerms(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill referencing an undefined term and one without a default.
---

{{term:missing}} and {{term:no_default}}.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[terms.no_default]
hermes = "~/.hermes/reports"
`)

	issues := Validate(loadForValidation(t, root))
	if _, ok := issueFor(issues, variant.CodeTermUnknown); !ok {
		t.Errorf("expected %s", variant.CodeTermUnknown)
	}
	if _, ok := issueFor(issues, variant.CodeTermDefaultRequired); !ok {
		t.Errorf("expected %s", variant.CodeTermDefaultRequired)
	}
}

func TestValidateWarnsOnOverridesForDisabledTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill shipping overrides for a target it disables.
---

<!-- symskills:block worker -->
Canonical.
<!-- /symskills:block -->
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), "[targets.claude]\nenabled = false\n")
	writeFile(t, filepath.Join(root, "overlays", "claude", "blocks", "worker.md"), "Claude worker.\n")

	issue, ok := issueFor(Validate(loadForValidation(t, root)), variant.CodeOverrideUnused)
	if !ok {
		t.Fatalf("expected %s", variant.CodeOverrideUnused)
	}
	if issue.Severity != "warning" {
		t.Errorf("dead overrides warn, they do not block: %s", issue.Severity)
	}
}

func TestValidateWarnsOnMisspelledRegionTarget(t *testing.T) {
	withKnownTargets(t, "claude", "hermes")
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill whose only-region names a target that does not exist.
---

<!-- symskills:only hermez -->
Text nobody will ever see.
<!-- /symskills:only -->
`)
	issue, ok := issueFor(Validate(loadForValidation(t, root)), variant.CodeTargetUnknown)
	if !ok {
		t.Fatalf("expected %s: a typo silently drops content for every harness", variant.CodeTargetUnknown)
	}
	if !strings.Contains(issue.Message, "hermez") {
		t.Errorf("message must name the typo: %s", issue.Message)
	}
}

func TestValidateReportsStructuralMarkerProblems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sweeper")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: sweeper
description: A skill with an unclosed block region.
---

<!-- symskills:block worker -->
Never closed.
`)
	issue, ok := issueFor(Validate(loadForValidation(t, root)), variant.CodeBlockUnclosed)
	if !ok {
		t.Fatalf("expected %s", variant.CodeBlockUnclosed)
	}
	if !strings.Contains(issue.Message, "line ") {
		t.Errorf("message must locate the marker: %s", issue.Message)
	}
	if issue.Path != "SKILL.md" {
		t.Errorf("path: got %q", issue.Path)
	}
}

// TestValidatePlainSkillReportsNoVariantIssues is the no-op guarantee at the
// validation layer: existing skills must not acquire new findings.
func TestValidatePlainSkillReportsNoVariantIssues(t *testing.T) {
	withKnownTargets(t, "claude", "hermes")
	root := filepath.Join(t.TempDir(), "plain")
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: plain
description: An ordinary skill with an HTML comment and braces.
category: developer-tools
---

<!-- an ordinary comment -->

Use {{mustache}} and {{term_without_colon}} freely.
`)
	writeFile(t, filepath.Join(root, "references", "notes.md"), "# Notes\n\nNothing special.\n")

	if issues := Validate(loadForValidation(t, root)); len(issues) != 0 {
		t.Errorf("expected no issues, got %+v", issues)
	}
}
