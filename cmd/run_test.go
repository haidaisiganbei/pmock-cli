package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haidaisiganbei/pmock-cli/internal/exitcode"
)

func testGlobals(base string) *globals {
	return &globals{
		format:    "json",
		baseURL:   base,
		accessKey: "AK",
		secretKey: "SK",
		timeout:   2 * time.Second,
	}
}

func TestRunOperationSuccess(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/open/api/phone/start" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body: %v", err)
		}
		_, _ = w.Write([]byte(`{"traceId":"trace-ok","code":0,"msg":"success","data":{"taskId":9},"status":"pending"}`))
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api")); err != nil {
		t.Fatal(err)
	}
	if gotBody["id"] != float64(123) {
		t.Fatalf("body = %v", gotBody)
	}
	if !strings.Contains(stdout.String(), `"traceId": "trace-ok"`) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunOperationBusinessFailure(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"trace-fail","code":42001,"msg":"手机不存在"}`))
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone remove")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "1")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	g := testGlobals(srv.URL + "/api")
	g.yes = true
	err := runOperation(cmd, sc, flagMap(sc), g)
	if code := exitcode.CodeOf(err); code != exitcode.Business {
		t.Fatalf("exit code = %d, want %d (%v)", code, exitcode.Business, err)
	}
	if !strings.Contains(stdout.String(), `"code": 42001`) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunOperationDestructiveNeedsYes(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone remove")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "1")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api"))
	if code := exitcode.CodeOf(err); code != exitcode.Usage {
		t.Fatalf("exit code = %d, want %d", code, exitcode.Usage)
	}
	if called {
		t.Fatal("destructive command reached the server without --yes")
	}
}

func TestRunOperationDryRun(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	g := testGlobals("https://example.test/api")
	g.dryRun = true
	if err := runOperation(cmd, sc, flagMap(sc), g); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"method": "POST"`) || !strings.Contains(out, `"id": 123`) {
		t.Fatalf("dry-run output = %s", out)
	}
}

func TestRunOperationAllowBusinessError(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"trace-fail","code":42001,"msg":"手机不存在"}`))
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone remove")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "1")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	g := testGlobals(srv.URL + "/api")
	g.yes = true
	g.allowBusinessError = true
	if err := runOperation(cmd, sc, flagMap(sc), g); err != nil {
		t.Fatalf("allow-business-error should return nil, got %v", err)
	}
}

func TestRunOperationMissingCredentials(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	t.Setenv("PMOCK_ACCESS_KEY", "")
	t.Setenv("PMOCK_SECRET_KEY", "")
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "1")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	g := &globals{format: "json", baseURL: "https://example.test/api", timeout: time.Second}
	err := runOperation(cmd, sc, flagMap(sc), g)
	if code := exitcode.CodeOf(err); code != exitcode.Usage {
		t.Fatalf("exit code = %d, want %d", code, exitcode.Usage)
	}
}

func TestRunOperationAuthError(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"trace-auth","code":10123,"msg":"无效的开发者编号"}`))
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "1")
	cmd.SetOut(&bytes.Buffer{})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api"))
	if code := exitcode.CodeOf(err); code != exitcode.Runtime {
		t.Fatalf("exit code = %d, want %d", code, exitcode.Runtime)
	}
	if !strings.Contains(stderr.String(), "Hint:") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}
