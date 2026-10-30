package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpToolByName(t *testing.T, tools []*mcpTool, name string) *mcpTool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", res.Content[0])
	}
	return tc.Text
}

func TestBuildMCPTools(t *testing.T) {
	tools := buildMCPTools(testCatalog(t))
	if len(tools) != 12 {
		t.Fatalf("tools = %d, want 12", len(tools))
	}
	total := 0
	paths := map[string]bool{}
	for _, tool := range tools {
		if len(tool.Actions) == 0 {
			t.Errorf("tool %s has no actions", tool.Name)
		}
		for _, a := range tool.Actions {
			total++
			key := strings.Join(a.Path, " ")
			if paths[key] {
				t.Errorf("path %q appears in more than one action", key)
			}
			paths[key] = true
		}
	}
	if total != 63 {
		t.Fatalf("actions = %d, want 63", total)
	}
	if !paths["phone file upload"] {
		t.Error("phone file upload action is missing")
	}
	if !paths["auth status"] || !paths["config init"] {
		t.Error("account actions are missing")
	}
}

func TestBuildCLIArgs(t *testing.T) {
	args, err := buildCLIArgs([]string{"phone", "action", "click"}, map[string]any{
		"code":     "ABC",
		"pos-type": float64(2),
		"pos":      []any{float64(10), float64(20)},
		"yes":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"phone", "action", "click", "--code=ABC", "--pos=10,20", "--pos-type=2", "--yes=true"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", args, want)
	}

	args, err = buildCLIArgs([]string{"rpa", "task", "add"}, map[string]any{
		"template-id": float64(9),
		"json":        map[string]any{"taskList": []any{map[string]any{"phoneId": 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), `--json={"taskList":[{"phoneId":1}]}`) {
		t.Fatalf("json arg = %v", args)
	}

	if _, err := buildCLIArgs([]string{"phone", "list"}, map[string]any{"bad name": 1}); err == nil {
		t.Fatal("expected an error for an invalid flag name")
	}
}

func TestBuildCLIArgsLargeIDs(t *testing.T) {
	// 2^53 is the largest float64 integer with no gaps; anything above it may
	// already be rounded by the MCP SDK, so it must be rejected, not truncated.
	const bigID = "9007199254740993"
	rounded := float64(1<<53 + 2)

	for _, id := range []float64{float64(1 << 53), rounded} {
		if _, err := buildCLIArgs([]string{"phone", "start"}, map[string]any{"id": id}); err == nil ||
			!strings.Contains(err.Error(), "quoted strings") {
			t.Fatalf("numeric id %v err = %v, want a precision error", id, err)
		}
	}

	args, err := buildCLIArgs([]string{"phone", "start"}, map[string]any{"id": bigID})
	if err != nil {
		t.Fatal(err)
	}
	if want := "--id=" + bigID; strings.Join(args, " ") != "phone start "+want {
		t.Fatalf("string id args = %v", args)
	}

	args, err = buildCLIArgs([]string{"phone", "start"}, map[string]any{"id": json.Number(bigID)})
	if err != nil {
		t.Fatal(err)
	}
	if want := "--id=" + bigID; strings.Join(args, " ") != "phone start "+want {
		t.Fatalf("json.Number id args = %v", args)
	}

	_, err = buildCLIArgs([]string{"rpa", "task", "add"}, map[string]any{
		"json": map[string]any{"taskList": []any{map[string]any{"phoneId": rounded}}},
	})
	if err == nil || !strings.Contains(err.Error(), "json.taskList[0].phoneId") {
		t.Fatalf("nested numeric id err = %v, want a precision error with a path", err)
	}

	args, err = buildCLIArgs([]string{"rpa", "task", "add"}, map[string]any{
		"json": map[string]any{"taskList": []any{map[string]any{"phoneId": json.Number(bigID)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), `"phoneId":`+bigID) {
		t.Fatalf("nested json.Number args = %v", args)
	}
}

func TestMCPCallToolSuccess(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	t.Setenv("PMOCK_ACCESS_KEY", "AK")
	t.Setenv("PMOCK_SECRET_KEY", "SK")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"mcp-ok","code":200,"msg":"查询成功","data":{"rows":[{"id":1}]}}`))
	}))
	defer srv.Close()
	t.Setenv("PMOCK_BASE_URL", srv.URL+"/api")

	catalog := testCatalog(t)
	s := &mcpServer{catalog: catalog, tools: buildMCPTools(catalog)}
	tool := mcpToolByName(t, s.tools, "pmock_phone")
	res, _, err := s.callTool(context.Background(), tool, mcpArgs{
		Action: "list",
		Params: map[string]any{"page-size": float64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "mcp-ok") {
		t.Fatalf("text = %s", resultText(t, res))
	}
}

func TestMCPCallToolBusinessError(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	t.Setenv("PMOCK_ACCESS_KEY", "AK")
	t.Setenv("PMOCK_SECRET_KEY", "SK")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"mcp-fail","code":42001,"msg":"手机不存在"}`))
	}))
	defer srv.Close()
	t.Setenv("PMOCK_BASE_URL", srv.URL+"/api")

	catalog := testCatalog(t)
	s := &mcpServer{catalog: catalog, tools: buildMCPTools(catalog)}
	tool := mcpToolByName(t, s.tools, "pmock_phone")
	res, _, err := s.callTool(context.Background(), tool, mcpArgs{
		Action: "remove",
		Params: map[string]any{"id": float64(1), "yes": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "42001") || !strings.Contains(text, "[Help:") {
		t.Fatalf("text = %s", text)
	}
}

func TestMCPUnknownAction(t *testing.T) {
	catalog := testCatalog(t)
	s := &mcpServer{catalog: catalog, tools: buildMCPTools(catalog)}
	tool := mcpToolByName(t, s.tools, "pmock_phone")
	res, _, err := s.callTool(context.Background(), tool, mcpArgs{Action: "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(resultText(t, res), "Valid actions") {
		t.Fatalf("res = %#v", res)
	}
}

func TestMCPHelp(t *testing.T) {
	catalog := testCatalog(t)
	s := &mcpServer{catalog: catalog}
	stdout, _, err := s.runHelp(context.Background(), []string{"phone", "file", "upload"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Upload a local file") {
		t.Fatalf("help = %s", stdout)
	}
}

func TestMCPConfigInitGuard(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	catalog := testCatalog(t)
	s := &mcpServer{catalog: catalog, tools: buildMCPTools(catalog)}
	tool := mcpToolByName(t, s.tools, "pmock_account")

	for _, params := range []map[string]any{nil, {"base-url": "https://example.test/api"}} {
		res, _, err := s.callTool(context.Background(), tool, mcpArgs{Action: "config-init", Params: params})
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || !strings.Contains(resultText(t, res), "params.access-key") {
			t.Fatalf("params=%v res=%s", params, resultText(t, res))
		}
	}
}
