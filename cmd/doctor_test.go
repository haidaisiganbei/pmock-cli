package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pmock/pmock-cli/internal/exitcode"
)

func runDoctor(t *testing.T, g *globals) (string, error) {
	t.Helper()
	cmd := newDoctorCmd(g)
	cmd.SetContext(context.Background())
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	err := cmd.RunE(cmd, nil)
	return out.String(), err
}

func TestDoctorSuccess(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	t.Setenv("PMOCK_ACCESS_KEY", "")
	t.Setenv("PMOCK_SECRET_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"doctor","code":0,"msg":"success","data":{"rows":[]}}`))
	}))
	defer srv.Close()

	out, err := runDoctor(t, &globals{format: "json", baseURL: srv.URL + "/api", accessKey: "AK", secretKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, "doctor") {
		t.Fatalf("out = %s", out)
	}
}

func TestDoctorMissingCredentials(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	t.Setenv("PMOCK_ACCESS_KEY", "")
	t.Setenv("PMOCK_SECRET_KEY", "")
	out, err := runDoctor(t, &globals{format: "json"})
	if code := exitcode.CodeOf(err); code != exitcode.Usage {
		t.Fatalf("exit code = %d (%v)", code, err)
	}
	if !strings.Contains(out, `"ok": false`) {
		t.Fatalf("out = %s", out)
	}
}

func TestDoctorAPIFailure(t *testing.T) {
	t.Setenv("PMOCK_CLI_CONFIG", t.TempDir()+"/none.json")
	t.Setenv("PMOCK_ACCESS_KEY", "")
	t.Setenv("PMOCK_SECRET_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	_, err := runDoctor(t, &globals{format: "json", baseURL: srv.URL + "/api", accessKey: "AK", secretKey: "SK"})
	if code := exitcode.CodeOf(err); code != exitcode.Runtime {
		t.Fatalf("exit code = %d (%v)", code, err)
	}
}
