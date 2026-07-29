package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-corekit/mcpserver"
)

func writeTestSkill(t *testing.T, dir string) {
	t.Helper()
	content := `---
name: test-skill
description: Test skill for unit tests
---
# Test Skill

Test description
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func writeTestProfile(t *testing.T, dir, name, skill string) {
	t.Helper()
	content := fmt.Sprintf("name = %q\n\n[links]\n%s = { skill = %q }\n", name, skill, skill)
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(content), 0644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

// parseText decodes a text content field that was produced by mcpJSON.
// After the serialization fix, text is a JSON string containing the
// serialized result (e.g. `"{\"skills\":[...]}"`). This helper
// unmarshals the JSON string first, then unmarshals the inner JSON.
func parseText(raw json.RawMessage, v any) error {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("unmarshal text string: %w", err)
	}
	return json.Unmarshal([]byte(text), v)
}

func TestRegisterExposesExpectedTools(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	Register(srv, Options{})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	names := map[string]bool{}
	for _, tool := range resp.Result.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"skills_list", "skills_inspect", "skills_validate", "skills_profile_list", "skills_profile_resolve", "skills_render_plan", "skills_install"} {
		if !names[want] {
			t.Fatalf("missing MCP tool %s in %#v", want, names)
		}
	}
}

func TestSkillsListEmptyLibrary(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "empty-lib")})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_list","arguments":{}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Skills []any `json:"skills"`
		Issues []any `json:"issues"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(result.Skills))
	}
}

func TestSkillsInspectMissingPath(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_inspect","arguments":{"path":"/nonexistent/path"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error for missing path")
	}
}

func TestSkillsValidateMissingPath(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_validate","arguments":{"path":"/nonexistent/path"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error for missing path")
	}
}

func TestSkillsRenderPlanMissingPath(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "lib"), RenderDir: filepath.Join(tmpDir, "render")})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"path":"/nonexistent/path"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error for missing path")
	}
}

func TestSkillsInstallMissingPath(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{
		LibraryDir: filepath.Join(tmpDir, "lib"),
		RenderDir:  filepath.Join(tmpDir, "render"),
		HomeDir:    tmpDir,
	})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_install","arguments":{"path":"/nonexistent/path"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error for missing path")
	}
}

func TestCallInspect(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()

	skillDir := filepath.Join(tmpDir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	skillContent := `---
name: test-skill
description: Test skill for unit tests
---
# Test Skill

Test description
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)

	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")})

	in := json.RawMessage(`{"path":"` + skillDir + `"}`)
	bundle, err := callInspect(context.Background(), srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")}, in)
	if err != nil {
		t.Fatalf("callInspect failed: %v", err)
	}
	if bundle == nil {
		t.Fatal("expected bundle, got nil")
	}

	in = json.RawMessage(`{"name":"nonexistent"}`)
	_, err = callInspect(context.Background(), srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")}, in)
	if err == nil {
		t.Fatal("expected error for nonexistent skill name")
	}

	in = json.RawMessage(`{}`)
	_, err = callInspect(context.Background(), srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")}, in)
	if err == nil {
		t.Fatal("expected error when path and name are missing")
	}
	if !strings.Contains(err.Error(), "path or name") {
		t.Fatalf("expected 'path or name' error, got: %v", err)
	}
}

func TestSkillsInspectRequiresPathOrName(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "lib")})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_inspect","arguments":{}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error when path and name are missing")
	}
}

func TestSkillsRenderPlanRejectsMalformedArguments(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{LibraryDir: filepath.Join(tmpDir, "lib"), RenderDir: filepath.Join(tmpDir, "render")})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"target":123}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}

	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error for malformed arguments")
	}
}

func TestOptionsDefaults(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	opts := Options{}
	Register(srv, opts)

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}
}

func TestSkillsProfileList(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	profilesDir := filepath.Join(tmpDir, "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "test.toml"), []byte("name = \"test-profile\"\ndescription = \"Test profile\"\n"), 0644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	Register(srv, Options{ProfilesDir: profilesDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_profile_list","arguments":{}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Profiles []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"profiles"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if len(result.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(result.Profiles))
	}
	if result.Profiles[0].Name != "test" {
		t.Errorf("expected profile name 'test', got %q", result.Profiles[0].Name)
	}
	if result.Profiles[0].Source != "global" {
		t.Errorf("expected profile source 'global', got %q", result.Profiles[0].Source)
	}
}

func TestSkillsProfileResolve(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	profilesDir := filepath.Join(tmpDir, "profiles")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}

	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)
	writeTestProfile(t, profilesDir, "test", "test-skill")

	Register(srv, Options{LibraryDir: libDir, ProfilesDir: profilesDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_profile_resolve","arguments":{"name":"test"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Skills []struct {
			Name  string `json:"name"`
			Skill string `json:"skill"`
		} `json:"skills"`
		Issues []any `json:"issues"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "test-skill" {
		t.Errorf("expected skill name 'test-skill', got %q", result.Skills[0].Name)
	}
	if result.Skills[0].Skill != "test-skill" {
		t.Errorf("expected skill 'test-skill', got %q", result.Skills[0].Skill)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestSkillsRenderPlanProfile(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	profilesDir := filepath.Join(tmpDir, "profiles")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}

	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)
	writeTestProfile(t, profilesDir, "test", "test-skill")

	Register(srv, Options{LibraryDir: libDir, ProfilesDir: profilesDir, RenderDir: renderDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"profile":"test"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var rendered []struct {
		Target string `json:"target"`
		Name   string `json:"name"`
	}
	if err := parseText(resp.Result.Content[0].Text, &rendered); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if len(rendered) != 4 {
		t.Fatalf("expected 4 rendered targets, got %d", len(rendered))
	}
	for _, item := range rendered {
		if item.Name != "test-skill" {
			t.Errorf("expected rendered name 'test-skill', got %q", item.Name)
		}
	}
}

func TestSkillsInstallProfileDryRun(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	profilesDir := filepath.Join(tmpDir, "profiles")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}

	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)
	writeTestProfile(t, profilesDir, "test", "test-skill")

	Register(srv, Options{LibraryDir: libDir, ProfilesDir: profilesDir, RenderDir: renderDir, HomeDir: tmpDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_install","arguments":{"profile":"test","dry_run":true}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var results []struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := parseText(resp.Result.Content[0].Text, &results); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != "planned" {
		t.Errorf("expected action 'planned', got %q", results[0].Action)
	}
	if results[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", results[0].Name)
	}
}

func TestSkillsRenderPlanProfileMissingSkill(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	profilesDir := filepath.Join(tmpDir, "profiles")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}

	writeTestProfile(t, profilesDir, "test", "missing-skill")

	Register(srv, Options{LibraryDir: libDir, ProfilesDir: profilesDir, RenderDir: renderDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"profile":"test"}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Skills []any `json:"skills"`
		Issues []any `json:"issues"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(result.Skills))
	}
	if len(result.Issues) == 0 {
		t.Errorf("expected issues for missing skill, got none")
	}
}

func TestSkillsListNonEmptyLibrary(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_list","arguments":{}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Skills []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"skills"`
		Issues []any `json:"issues"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", result.Skills[0].Name)
	}
}

func TestSkillsValidateWithIssues(t *testing.T) {
	// A skill with name>64 chars triggers a validation issue.
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "bad-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	content := "---\nname: " + strings.Repeat("x", 65) + "\ndescription: invalid\n---\n# Body\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	Register(srv, Options{LibraryDir: libDir})

	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_validate","arguments":{"path":"%s"}}}`, skillDir) + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Issues []any `json:"issues"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected validation issues for bad skill name, got none")
	}
}

func TestSkillsRenderPlanInvalidTarget(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir, RenderDir: renderDir})

	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"path":"%s","target":"invalid"}}}`, skillDir) + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error for invalid target")
	}
}

func TestSkillsRenderPlanSingleSkill(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir, RenderDir: renderDir, HomeDir: tmpDir})

	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"path":"%s"}}}`, skillDir) + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}
}

func TestSkillsInstallSingleSkillDryRun(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir, RenderDir: renderDir, HomeDir: tmpDir})

	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_install","arguments":{"path":"%s","dry_run":true}}}`, skillDir) + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Action != "planned" {
		t.Errorf("expected action 'planned', got %q", result.Action)
	}
	if result.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", result.Name)
	}
}

func TestSkillsInstallMissingPathOrName(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	Register(srv, Options{
		LibraryDir: filepath.Join(tmpDir, "lib"),
		RenderDir:  filepath.Join(tmpDir, "render"),
		HomeDir:    tmpDir,
	})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_install","arguments":{}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if resp.Error == nil && !resp.Result.IsError {
		t.Fatal("expected error when path and name are missing")
	}
}

func TestSkillsInstallProfileMissingSkill(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	profilesDir := filepath.Join(tmpDir, "profiles")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("create profiles dir: %v", err)
	}

	writeTestProfile(t, profilesDir, "test", "missing-skill")

	Register(srv, Options{LibraryDir: libDir, ProfilesDir: profilesDir, RenderDir: renderDir, HomeDir: tmpDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_install","arguments":{"profile":"test","dry_run":true}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	var result struct {
		Results []any `json:"results"`
		Issues  []any `json:"issues"`
	}
	if err := parseText(resp.Result.Content[0].Text, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(result.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Results))
	}
	if len(result.Issues) == 0 {
		t.Errorf("expected issues for missing skill, got none")
	}
}

// TestMCPTextIsJSONString verifies that tool results are serialized
// as JSON strings (TextContent.text is a string, not a raw object).
func TestMCPTextIsJSONString(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir})

	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_list","arguments":{}}}` + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	// Verify the text field is a JSON string (wrap response in object for valid JSON)
	type contentItem struct {
		Type string          `json:"type"`
		Text json.RawMessage `json:"text"`
	}
	type resultBlock struct {
		Content []contentItem `json:"content"`
	}
	type response struct {
		Result resultBlock `json:"result"`
	}
	var resp response
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response %q: %v", out.String(), err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Text must be a JSON string (starts with '"')
	text := resp.Result.Content[0].Text
	if len(text) == 0 || text[0] != '"' {
		t.Fatalf("expected Text to be a JSON string, got: %s", string(text))
	}
	if text[len(text)-1] != '"' {
		t.Fatalf("expected Text to be a JSON string ending with '\"', got: %s", string(text))
	}
}

// TestSkillsRenderPlanDryRunDoesNotWrite verifies that dry_run=true on
// skills_render_plan does not write files to the render directory.
func TestSkillsRenderPlanDryRunDoesNotWrite(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir, RenderDir: renderDir})

	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_render_plan","arguments":{"path":"%s","dry_run":true}}}`, skillDir) + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Verify the render directory has no written files
	entries, err := os.ReadDir(renderDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read render dir: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("expected empty render dir after dry_run, got %d entries", len(entries))
	}
}

// TestSkillsInstallDryRunDoesNotWriteRenderDir verifies that dry_run=true
// on skills_install does not write to the render directory.
func TestSkillsInstallDryRunDoesNotWriteRenderDir(t *testing.T) {
	srv := mcpserver.New("symskills", "test")
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	renderDir := filepath.Join(tmpDir, "render")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	skillDir := filepath.Join(libDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	writeTestSkill(t, skillDir)

	Register(srv, Options{LibraryDir: libDir, RenderDir: renderDir, HomeDir: tmpDir})

	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skills_install","arguments":{"path":"%s","dry_run":true}}}`, skillDir) + "\n"
	var out strings.Builder
	if err := srv.ServeIO(context.Background(), strings.NewReader(req), &out); err != nil {
		t.Fatalf("ServeIO: %v", err)
	}

	var resp struct {
		Result struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Verify the render directory has no written files
	entries, err := os.ReadDir(renderDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read render dir: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("expected empty render dir after dry_run, got %d entries", len(entries))
	}
}
