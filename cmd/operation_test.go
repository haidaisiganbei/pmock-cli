package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/haidaisiganbei/pmock-cli/internal/exitcode"
	"github.com/haidaisiganbei/pmock-cli/internal/spec"
)

func testCatalog(t *testing.T) *spec.Catalog {
	t.Helper()
	docFile, err := os.Open("../openapi/pmock-openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer docFile.Close()
	doc, err := spec.Load(docFile)
	if err != nil {
		t.Fatal(err)
	}
	overlayFile, err := os.Open("../openapi/overlay.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer overlayFile.Close()
	overlay, err := spec.LoadOverlay(overlayFile)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := spec.BuildCatalog(doc, overlay)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func testCommand(t *testing.T, catalog *spec.Catalog, path string) *spec.Command {
	t.Helper()
	for _, c := range catalog.Commands {
		if c.FullPath() == path {
			return c
		}
	}
	t.Fatalf("command %q not found", path)
	return nil
}

func prepare(t *testing.T, sc *spec.Command) *cobra.Command {
	t.Helper()
	cmd := newOperationCmd(sc, &globals{})
	cmd.SetContext(context.Background())
	return cmd
}

func TestBuildRequestStart(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	if err := cmd.Flags().Set("id", "123"); err != nil {
		t.Fatal(err)
	}
	path, query, body, err := buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if path != "/open/api/phone/start" {
		t.Errorf("path = %q", path)
	}
	if len(query) != 0 {
		t.Errorf("query = %v", query)
	}
	if got := body["id"]; got != int64(123) {
		t.Errorf("body id = %#v", got)
	}
}

func TestBuildRequestQueryAndDuplicateBody(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "app list")
	cmd := prepare(t, sc)
	if err := cmd.Flags().Set("page-size", "5"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("apk-name", "tiktok"); err != nil {
		t.Fatal(err)
	}
	_, query, body, err := buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if got := query.Get("pageSize"); got != "5" {
		t.Errorf("query pageSize = %q", got)
	}
	if got := body["pageSize"]; got != int64(5) {
		t.Errorf("body pageSize = %#v", got)
	}
	if got := body["apkName"]; got != "tiktok" {
		t.Errorf("body apkName = %#v", got)
	}
}

func TestBuildRequestPathParam(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "rpa task get")
	cmd := prepare(t, sc)
	if err := cmd.Flags().Set("id", "42"); err != nil {
		t.Fatal(err)
	}
	path, _, body, err := buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if path != "/open/api/rpa/task/getInfo/42" {
		t.Errorf("path = %q", path)
	}
	if body != nil {
		t.Errorf("body = %v, want nil", body)
	}
}

func TestBuildRequestMissingRequired(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	_, _, _, err := buildRequest(cmd, sc, flagMap(sc))
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := exitcode.CodeOf(err); code != exitcode.Usage {
		t.Fatalf("exit code = %d, want %d", code, exitcode.Usage)
	}
}

func TestBuildRequestJSONSatisfiesRequired(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	if err := cmd.Flags().Set("json", `{"id":123}`); err != nil {
		t.Fatal(err)
	}
	_, _, body, err := buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if body["id"] == nil {
		t.Fatalf("body = %v", body)
	}
}

func TestBuildRequestJSONFlagsOverride(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	if err := cmd.Flags().Set("json", `{"id":1,"configId":9}`); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("id", "2"); err != nil {
		t.Fatal(err)
	}
	_, _, body, err := buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if got := body["id"]; got != int64(2) {
		t.Errorf("flag should override JSON: %#v", got)
	}
	if got := body["configId"]; got == nil {
		t.Errorf("JSON-only field was lost: %v", body)
	}
}

func TestBuildRequestRejectsJSONArray(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "phone start")
	cmd := prepare(t, sc)
	if err := cmd.Flags().Set("json", `[1,2]`); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := buildRequest(cmd, sc, flagMap(sc)); err == nil {
		t.Fatal("expected an error for a JSON array body")
	}
}

func TestScreenRemovesScreenshotByDefault(t *testing.T) {
	sc := testCommand(t, testCatalog(t), "phone screen")
	cmd := prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	_, _, body, err := buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if body["isRemove"] != true {
		t.Fatalf("isRemove = %#v, want true", body["isRemove"])
	}

	cmd = prepare(t, sc)
	_ = cmd.Flags().Set("id", "123")
	_ = cmd.Flags().Set("is-remove", "false")
	_, _, body, err = buildRequest(cmd, sc, flagMap(sc))
	if err != nil {
		t.Fatal(err)
	}
	if body["isRemove"] != false {
		t.Fatalf("explicit isRemove = %#v, want false", body["isRemove"])
	}
}
