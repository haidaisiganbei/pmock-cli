package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pmock/pmock-cli/internal/client"
)

func env(data string) *client.Envelope {
	return &client.Envelope{
		TraceID: "trace-1",
		Code:    0,
		Msg:     "success",
		Data:    json.RawMessage(data),
		Status:  "finish",
	}
}

func TestRenderJSONKeepsHTMLCharacters(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, env(`{"url":"https://x.test?a=1&b=2"}`), Options{Format: "json"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, `\u0026`) {
		t.Fatalf("JSON escaped &: %s", got)
	}
	if !strings.Contains(got, "https://x.test?a=1&b=2") {
		t.Fatalf("unexpected JSON: %s", got)
	}
	if strings.Count(got, "\n") < 5 {
		t.Fatalf("expected indented JSON: %s", got)
	}
}

func TestRenderCompact(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, env(`{"ok":true}`), Options{Format: "json", Compact: true}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); strings.Count(got, "\n") != 1 {
		t.Fatalf("compact output should be one line: %q", got)
	}
}

func TestRenderTableFromRows(t *testing.T) {
	var buf bytes.Buffer
	data := `{"rows":[{"name":"张三","id":1},{"name":"李四","id":22}]}`
	if err := Render(&buf, env(data), Options{Format: "table"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"NAME", "ID", "张三", "李四", "2 rows"} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPrettyFailure(t *testing.T) {
	var buf bytes.Buffer
	e := env(`null`)
	e.Code = 10123
	e.Msg = "无效的开发者编号"
	if err := Render(&buf, e, Options{Format: "pretty"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "FAILED") || !strings.Contains(got, "10123") {
		t.Fatalf("unexpected pretty output:\n%s", got)
	}
}
