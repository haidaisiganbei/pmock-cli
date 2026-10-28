package spec

import (
	"os"
	"testing"
)

func loadCatalog(t *testing.T) *Catalog {
	t.Helper()
	docFile, err := os.Open("../../openapi/pmock-openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer docFile.Close()
	doc, err := Load(docFile)
	if err != nil {
		t.Fatal(err)
	}
	overlayFile, err := os.Open("../../openapi/overlay.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer overlayFile.Close()
	overlay, err := LoadOverlay(overlayFile)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := BuildCatalog(doc, overlay)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func find(t *testing.T, catalog *Catalog, path string) *Command {
	t.Helper()
	for _, c := range catalog.Commands {
		if c.FullPath() == path {
			return c
		}
	}
	t.Fatalf("command %q not found", path)
	return nil
}

func flag(t *testing.T, c *Command, name string) *Flag {
	t.Helper()
	for _, f := range c.Flags {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("flag --%s not found on %s", name, c.FullPath())
	return nil
}

func TestCatalogCoversEveryOperation(t *testing.T) {
	catalog := loadCatalog(t)
	if got, want := len(catalog.Commands), 60; got != want {
		t.Fatalf("commands = %d, want %d", got, want)
	}
	seen := map[string]bool{}
	for _, c := range catalog.Commands {
		if seen[c.FullPath()] {
			t.Fatalf("duplicate command path %q", c.FullPath())
		}
		seen[c.FullPath()] = true
		if c.Summary == "" {
			t.Errorf("%s has no summary", c.FullPath())
		}
	}
}

func TestOverlayCoversEveryOperation(t *testing.T) {
	docFile, err := os.Open("../../openapi/pmock-openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer docFile.Close()
	doc, err := Load(docFile)
	if err != nil {
		t.Fatal(err)
	}
	overlayFile, err := os.Open("../../openapi/overlay.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer overlayFile.Close()
	overlay, err := LoadOverlay(overlayFile)
	if err != nil {
		t.Fatal(err)
	}

	ops := map[string]bool{}
	for _, ref := range doc.Operations() {
		ops[ref.Key()] = true
	}
	entries := map[string]bool{}
	for _, c := range overlay.Commands {
		if entries[c.Op] {
			t.Errorf("duplicate overlay entry %s", c.Op)
		}
		entries[c.Op] = true
		if !ops[c.Op] {
			t.Errorf("overlay entry %s does not match any operation", c.Op)
		}
	}
	for key := range ops {
		if !entries[key] {
			t.Errorf("operation %s has no overlay entry", key)
		}
	}
}

func TestKnownCommands(t *testing.T) {
	catalog := loadCatalog(t)

	start := find(t, catalog, "phone start")
	if start.Method != "POST" || start.Path != "/open/api/phone/start" {
		t.Fatalf("unexpected start command: %+v", start)
	}
	if f := flag(t, start, "id"); !f.Required || f.In != "body" || f.Type != TypeInteger {
		t.Fatalf("unexpected id flag: %+v", f)
	}

	create := find(t, catalog, "phone create")
	for _, name := range []string{"spu-id", "sku-id", "num"} {
		if !flag(t, create, name).Required {
			t.Errorf("--%s should be required", name)
		}
	}
	if f := flag(t, create, "tag-ids"); f.Type != TypeIntArray {
		t.Errorf("tag-ids type = %q", f.Type)
	}
	if create.Risk != RiskCostly {
		t.Errorf("create risk = %q", create.Risk)
	}

	click := find(t, catalog, "phone action click")
	if f := flag(t, click, "code"); !f.Required {
		t.Error("click --code should be required")
	}
	if f := flag(t, click, "pos"); f.Type != TypeIntArray {
		t.Errorf("pos type = %q", f.Type)
	}
	if f := flag(t, click, "query"); f.Type != TypeStringArray {
		t.Errorf("query type = %q", f.Type)
	}

	getTask := find(t, catalog, "rpa task get")
	if f := flag(t, getTask, "id"); !f.Required || f.In != "path" {
		t.Fatalf("rpa task get --id: %+v", f)
	}

	upload := find(t, catalog, "phone file upload-init")
	if !upload.Internal {
		t.Error("upload-init should be hidden")
	}
}

func TestKebab(t *testing.T) {
	cases := map[string]string{
		"spuId":          "spu-id",
		"realResolution": "real-resolution",
		"pageSize":       "page-size",
		"nameOrCode":     "name-or-code",
		"withG":          "with-g",
		"apkPkg":         "apk-pkg",
		"adbIp":          "adb-ip",
		"URLValue":       "url-value",
	}
	for in, want := range cases {
		if got := Kebab(in); got != want {
			t.Errorf("Kebab(%q) = %q, want %q", in, got, want)
		}
	}
}
