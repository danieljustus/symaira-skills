package mcptools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-corekit/mcpserver"
)

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
	for _, want := range []string{"skills_list", "skills_inspect", "skills_validate", "skills_render_plan", "skills_install"} {
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
	if err := json.Unmarshal(resp.Result.Content[0].Text, &result); err != nil {
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
