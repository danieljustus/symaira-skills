package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-skills/internal/skill"
)

func TestRenderTargetAppliesOverlayAndTargetFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: overlaid
description: Base description for render tests.
license: Apache-2.0
metadata:
  workflow: base
---

# Base Body

Use the base workflow.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[skill]
name = "overlaid"
version = "0.2.0"

[targets.opencode]
enabled = true
alias = "overlaid-open"
description = "OpenCode-specific description."
`)
	writeFile(t, filepath.Join(root, "overlays", "opencode", "prepend.md"), "## OpenCode Note\n\nLoad guard skills first.\n")
	writeFile(t, filepath.Join(root, "overlays", "opencode", "append.md"), "## OpenCode Tail\n\nReport next skill.\n")
	writeFile(t, filepath.Join(root, "overlays", "opencode", "frontmatter.toml"), `[metadata]
workflow = "github"
audience = "maintainers"
`)

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := RenderTarget(bundle, TargetOpenCode)
	if err != nil {
		t.Fatalf("RenderTarget: %v", err)
	}

	if rendered.Name != "overlaid-open" {
		t.Fatalf("rendered name: want alias, got %q", rendered.Name)
	}
	if rendered.Frontmatter.Description != "OpenCode-specific description." {
		t.Fatalf("description override missing: %q", rendered.Frontmatter.Description)
	}
	if rendered.Frontmatter.Compatibility != "opencode" {
		t.Fatalf("compatibility: want opencode, got %q", rendered.Frontmatter.Compatibility)
	}
	if rendered.Frontmatter.Metadata["audience"] != "maintainers" {
		t.Fatalf("overlay metadata missing: %#v", rendered.Frontmatter.Metadata)
	}
	if !strings.Contains(rendered.SkillMD, "## OpenCode Note") || !strings.Contains(rendered.SkillMD, "## OpenCode Tail") {
		t.Fatalf("overlay body fragments missing:\n%s", rendered.SkillMD)
	}
}

func TestRenderAllWritesHarnessReadableSkillFolders(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: basic
description: Basic render fixture.
---

Body.
`)
	writeFile(t, filepath.Join(root, "scripts", "helper.sh"), "#!/bin/sh\n")

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "rendered")
	results, errs := RenderAll(bundle, out, []Target{TargetOpenCode, TargetClaude, TargetCodex, TargetHermes})
	if len(errs) != 0 {
		t.Fatalf("RenderAll errors: %v", errs)
	}
	if len(results) != 4 {
		t.Fatalf("want 4 rendered targets, got %d", len(results))
	}
	for _, result := range results {
		if _, err := os.Stat(filepath.Join(result.Path, "SKILL.md")); err != nil {
			t.Fatalf("%s SKILL.md missing: %v", result.Target, err)
		}
		if _, err := os.Stat(filepath.Join(result.Path, "scripts", "helper.sh")); err != nil {
			t.Fatalf("%s copied support file missing: %v", result.Target, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "codex", "basic", "agents", "openai.yaml")); err != nil {
		t.Fatalf("codex metadata file missing: %v", err)
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

func TestRenderTargetRejectsHostileResolvedNames(t *testing.T) {
	cases := []struct {
		name            string
		frontmatterName string
		manifest        string
		overlay         string
	}{
		{
			name:            "alias_with_path_traversal",
			frontmatterName: "safe",
			manifest: `[skill]
name = "safe"
version = "0.1.0"

[targets.opencode]
enabled = true
alias = "../../evil"
`,
		},
		{
			name:            "manifest_name_with_separator",
			frontmatterName: "safe",
			manifest: `[skill]
name = "evil/name"
version = "0.1.0"

[targets.opencode]
enabled = true
`,
		},
		{
			name:            "overlay_name_with_path_traversal",
			frontmatterName: "safe",
			manifest: `[skill]
name = "safe"
version = "0.1.0"

[targets.opencode]
enabled = true
`,
			overlay: `name = "../evil"
`,
		},
		{
			name:            "frontmatter_name_absolute_path",
			frontmatterName: "/etc/evil",
			manifest: `[skill]
version = "0.1.0"

[targets.opencode]
enabled = true
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "SKILL.md"), fmt.Sprintf(`---
name: %s
description: Test.
---

Body.
`, tc.frontmatterName))
			writeFile(t, filepath.Join(root, "symskills.toml"), tc.manifest)
			if tc.overlay != "" {
				writeFile(t, filepath.Join(root, "overlays", "opencode", "frontmatter.toml"), tc.overlay)
			}

			bundle, err := skill.LoadBundle(root)
			if err != nil {
				t.Fatal(err)
			}

			_, err = RenderTarget(bundle, TargetOpenCode)
			if err == nil {
				t.Fatal("expected error for hostile resolved name")
			}
		})
	}
}

func TestRenderAllReportsPerTargetErrors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: error-test
description: Test.
---

Body.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[skill]
name = "error-test"
version = "0.1.0"

[targets.opencode]
enabled = false

[targets.claude]
enabled = true
alias = "../../evil"

[targets.codex]
enabled = true

[targets.hermes]
enabled = false
`)

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "rendered")
	results, errs := RenderAll(bundle, out, []Target{TargetOpenCode, TargetClaude, TargetCodex, TargetHermes})
	if len(results) != 1 {
		t.Fatalf("want 1 successful render (codex), got %d", len(results))
	}
	if results[0].Target != TargetCodex {
		t.Fatalf("want codex success, got %s", results[0].Target)
	}
	if len(errs) != 3 {
		t.Fatalf("want 3 errors (opencode disabled, claude hostile, hermes disabled), got %d: %v", len(errs), errs)
	}
}

func TestParseTarget(t *testing.T) {
	valid := []Target{TargetOpenCode, TargetClaude, TargetCodex, TargetHermes}
	for _, target := range valid {
		got, err := ParseTarget(string(target))
		if err != nil {
			t.Fatalf("expected target %q to parse successfully, got: %v", target, err)
		}
		if got != target {
			t.Errorf("ParseTarget(%q) = %q, want %q", target, got, target)
		}
	}

	invalid := []string{"", "invalid-target", "open-code", "CLAUDE"}
	for _, s := range invalid {
		_, err := ParseTarget(s)
		if err == nil {
			t.Fatalf("expected error parsing invalid target %q, but got nil", s)
		}
	}
}

func TestRenderTargetCarriesProvenanceMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: meta-test
description: Meta test.
---

Body.
`)
	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := RenderTarget(bundle, TargetOpenCode, RenderMeta{Source: "project", Profile: "default"})
	if err != nil {
		t.Fatalf("RenderTarget: %v", err)
	}
	if rendered.Source != "project" {
		t.Fatalf("source: want project, got %q", rendered.Source)
	}
	if rendered.Profile != "default" {
		t.Fatalf("profile: want default, got %q", rendered.Profile)
	}

	var decoded Rendered
	if err := jsonRoundTrip(rendered, &decoded); err != nil {
		t.Fatalf("JSON round-trip: %v", err)
	}
	if decoded.Source != "project" || decoded.Profile != "default" {
		t.Fatalf("JSON decoded metadata mismatch: %+v", decoded)
	}
}

func TestRenderTargetWithoutMetaOmitsProvenanceFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: no-meta
description: No meta.
---

Body.
`)
	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := RenderTarget(bundle, TargetOpenCode)
	if err != nil {
		t.Fatalf("RenderTarget: %v", err)
	}
	if rendered.Source != "" || rendered.Profile != "" {
		t.Fatalf("expected empty source/profile, got %+v", rendered)
	}

	var decoded Rendered
	if err := jsonRoundTrip(rendered, &decoded); err != nil {
		t.Fatalf("JSON round-trip: %v", err)
	}
	if decoded.Source != "" || decoded.Profile != "" {
		t.Fatalf("JSON decoded metadata should be empty, got %+v", decoded)
	}
}

func jsonRoundTrip(in, out any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func TestWriteRenderedPreservesInstallMarker(t *testing.T) {
	// Regression test for #65: re-rendering a directory that contains
	// a .symskills.json marker must not wipe it (symlink-mode installs
	// share the render-cache directory).
	tmp := t.TempDir()
	marker := filepath.Join(tmp, ".symskills.json")
	if err := os.WriteFile(marker, []byte(`{"installed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), "---\nname: marker-test\ndescription: test\n---\n# Body\n")

	item := Rendered{Name: "marker-test", SkillMD: "---\nname: marker-test\ndescription: test\n---\n# Body\n"}
	if err := writeRendered(root, tmp, item, TargetOpenCode); err != nil {
		t.Fatalf("writeRendered: %v", err)
	}

	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Fatal(".symskills.json was wiped by re-render — regression of #65")
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"installed":true}` {
		t.Fatalf("marker content changed: got %q", string(data))
	}
}

// --- Regression tests for #80: overlay path traversal hardening ---

func TestRenderTargetRejectsTraversalOverlayPrepend(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: traversal-prepend
description: Test traversal in prepend.
---
Body.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[skill]
name = "traversal-prepend"

[targets.opencode]
enabled = true
prepend = "../../evil.md"
`)

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = RenderTarget(bundle, TargetOpenCode)
	if err == nil {
		t.Fatal("expected error for traversal prepend, got nil")
	}
	if !strings.Contains(err.Error(), "escapes") && !strings.Contains(err.Error(), "must be relative") {
		t.Fatalf("expected containment error, got: %v", err)
	}
}

func TestRenderTargetRejectsTraversalOverlayAppend(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: traversal-append
description: Test traversal in append.
---
Body.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[skill]
name = "traversal-append"

[targets.opencode]
enabled = true
append = "../evil.md"
`)

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = RenderTarget(bundle, TargetOpenCode)
	if err == nil {
		t.Fatal("expected error for traversal append, got nil")
	}
	if !strings.Contains(err.Error(), "escapes") && !strings.Contains(err.Error(), "must be relative") {
		t.Fatalf("expected containment error, got: %v", err)
	}
}

func TestRenderTargetRejectsAbsoluteOverlayPrepend(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: absolute-prepend
description: Test absolute path in prepend.
---
Body.
`)
	writeFile(t, filepath.Join(root, "symskills.toml"), `[skill]
name = "absolute-prepend"

[targets.opencode]
enabled = true
prepend = "/etc/passwd"
`)

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = RenderTarget(bundle, TargetOpenCode)
	if err == nil {
		t.Fatal("expected error for absolute prepend, got nil")
	}
	if !strings.Contains(err.Error(), "must be relative") {
		t.Fatalf("expected 'must be relative' error, got: %v", err)
	}
}

func TestRenderTargetAcceptsValidConfiguredOverlay(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), `---
name: valid-configured
description: Valid configured overlay.
---
Base body.
`)
	writeFile(t, filepath.Join(root, "custom-prepend.md"), "## Custom Prepend\n\nExtra content.\n")
	writeFile(t, filepath.Join(root, "symskills.toml"), `[skill]
name = "valid-configured"

[targets.opencode]
enabled = true
prepend = "custom-prepend.md"
`)

	bundle, err := skill.LoadBundle(root)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := RenderTarget(bundle, TargetOpenCode)
	if err != nil {
		t.Fatalf("expected valid overlay to succeed, got: %v", err)
	}
	if !strings.Contains(rendered.SkillMD, "## Custom Prepend") {
		t.Fatalf("expected custom prepend in output, got:\n%s", rendered.SkillMD)
	}
}
