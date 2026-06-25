package mcptools

import (
	"context"
	"encoding/json"
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
