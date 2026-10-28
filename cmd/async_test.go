package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pmock/pmock-cli/internal/exitcode"
)

func decodeResult(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	return out
}

func TestWaitPhoneStart(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/open/api/phone/start":
			_, _ = w.Write([]byte(`{"traceId":"start","code":0,"msg":"success","data":{"taskId":1,"failed":false},"status":"pending"}`))
		case "/api/open/api/phone/task/status":
			if r.URL.Query().Get("operateType") != "powerOn" {
				t.Errorf("operateType = %q", r.URL.Query().Get("operateType"))
			}
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				_, _ = w.Write([]byte(`{"traceId":"s1","code":0,"msg":"success","data":{"taskStatus":1,"phoneStatus":41}}`))
				return
			}
			_, _ = w.Write([]byte(`{"traceId":"s2","code":0,"msg":"success","data":{"taskStatus":2,"phoneStatus":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	_ = cmd.Flags().Set("wait", "true")
	_ = cmd.Flags().Set("wait-timeout", "2s")
	_ = cmd.Flags().Set("poll-interval", "50ms")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api")); err != nil {
		t.Fatal(err)
	}
	out := decodeResult(t, &stdout)
	data := out["data"].(map[string]any)
	wait := data["wait"].(map[string]any)
	if wait["ok"] != true {
		t.Fatalf("wait = %#v", wait)
	}
	if wait["attempts"].(float64) != 2 {
		t.Errorf("attempts = %v", wait["attempts"])
	}
	if data["taskId"].(float64) != 1 {
		t.Errorf("taskId = %v", data["taskId"])
	}
}

func TestWaitAction(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/open/api/phone/action/click":
			_, _ = w.Write([]byte(`{"traceId":"click","code":0,"msg":"success","data":5}`))
		case "/api/open/api/phone/action/task/status":
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				_, _ = w.Write([]byte(`{"traceId":"a1","code":0,"msg":"success","data":{"taskStatus":1}}`))
				return
			}
			_, _ = w.Write([]byte(`{"traceId":"a2","code":0,"msg":"success","data":{"taskStatus":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone action click")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("code", "ABC")
	_ = cmd.Flags().Set("pos-type", "2")
	_ = cmd.Flags().Set("pos", "10,20")
	_ = cmd.Flags().Set("wait", "true")
	_ = cmd.Flags().Set("wait-timeout", "2s")
	_ = cmd.Flags().Set("poll-interval", "50ms")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api")); err != nil {
		t.Fatal(err)
	}
	out := decodeResult(t, &stdout)
	data := out["data"].(map[string]any)
	if data["taskId"].(float64) != 5 {
		t.Fatalf("data = %#v", data)
	}
	wait := data["wait"].(map[string]any)
	if wait["ok"] != true || wait["taskStatus"].(float64) != 2 {
		t.Fatalf("wait = %#v", wait)
	}
}

func TestWaitFailure(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/open/api/phone/start" {
			_, _ = w.Write([]byte(`{"traceId":"start","code":0,"msg":"success","data":{"taskId":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"traceId":"s","code":0,"msg":"success","data":{"taskStatus":3,"phoneStatus":4}}`))
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	_ = cmd.Flags().Set("wait", "true")
	_ = cmd.Flags().Set("poll-interval", "50ms")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api"))
	if code := exitcode.CodeOf(err); code != exitcode.Business {
		t.Fatalf("exit code = %d, want %d (%v)", code, exitcode.Business, err)
	}
	out := decodeResult(t, &stdout)
	wait := out["data"].(map[string]any)["wait"].(map[string]any)
	if wait["ok"] != false || wait["error"] == "" {
		t.Fatalf("wait = %#v", wait)
	}
}

func TestWaitTimeout(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/open/api/phone/start" {
			_, _ = w.Write([]byte(`{"traceId":"start","code":0,"msg":"success","data":{"taskId":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"traceId":"s","code":0,"msg":"success","data":{"taskStatus":1,"phoneStatus":41}}`))
	}))
	defer srv.Close()

	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	_ = cmd.Flags().Set("wait", "true")
	_ = cmd.Flags().Set("wait-timeout", "120ms")
	_ = cmd.Flags().Set("poll-interval", "50ms")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	err := runOperation(cmd, sc, flagMap(sc), testGlobals(srv.URL+"/api"))
	if code := exitcode.CodeOf(err); code != exitcode.Runtime {
		t.Fatalf("exit code = %d, want %d (%v)", code, exitcode.Runtime, err)
	}
	out := decodeResult(t, &stdout)
	wait := out["data"].(map[string]any)["wait"].(map[string]any)
	if wait["timedOut"] != true {
		t.Fatalf("wait = %#v", wait)
	}
}
