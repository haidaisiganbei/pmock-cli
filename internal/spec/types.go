// Package spec parses the vendored PMOCK OpenAPI document and turns it into
// the CLI command catalog.
//
// The OpenAPI file is the single source of truth: command paths, flags,
// descriptions, required fields and enum hints all come from it. The small
// overlay.yaml only supplies product decisions the spec cannot express
// (curated command names, aliases, risk level, hidden internal operations).
package spec

import (
	"fmt"
	"io"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is the subset of OpenAPI 3.0 used by this CLI.
type Document struct {
	OpenAPI    string                           `yaml:"openapi"`
	Info       Info                             `yaml:"info"`
	Paths      map[string]map[string]*Operation `yaml:"paths"`
	Components Components                       `yaml:"components"`
}

// Info is the OpenAPI info block.
type Info struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// Components holds reusable schemas.
type Components struct {
	Schemas map[string]*Schema `yaml:"schemas"`
}

// Operation is one HTTP operation.
type Operation struct {
	Summary     string              `yaml:"summary"`
	Description string              `yaml:"description"`
	Tags        []string            `yaml:"tags"`
	Parameters  []Parameter         `yaml:"parameters"`
	RequestBody *RequestBody        `yaml:"requestBody"`
	Responses   map[string]Response `yaml:"responses"`
	Deprecated  bool                `yaml:"deprecated"`
}

// Parameter is a path or query parameter. Header parameters (the doc
// generator's placeholder `chameleon`) are parsed but ignored by the catalog.
type Parameter struct {
	Name        string  `yaml:"name"`
	In          string  `yaml:"in"`
	Description string  `yaml:"description"`
	Required    bool    `yaml:"required"`
	Schema      *Schema `yaml:"schema"`
}

// RequestBody describes a JSON request body.
type RequestBody struct {
	Required bool                 `yaml:"required"`
	Content  map[string]MediaType `yaml:"content"`
}

// MediaType wraps a schema.
type MediaType struct {
	Schema *Schema `yaml:"schema"`
}

// Response is an HTTP response.
type Response struct {
	Description string               `yaml:"description"`
	Content     map[string]MediaType `yaml:"content"`
}

// Schema is the subset of JSON Schema used by the PMOCK spec. It covers
// primitives, arrays and $ref objects; the spec uses no allOf/oneOf/anyOf.
type Schema struct {
	Ref         string             `yaml:"$ref"`
	Type        string             `yaml:"type"`
	Format      string             `yaml:"format"`
	Description string             `yaml:"description"`
	Required    []string           `yaml:"required"`
	Properties  map[string]*Schema `yaml:"properties"`
	Items       *Schema            `yaml:"items"`
	Enum        []any              `yaml:"enum"`
	Example     any                `yaml:"example"`
	Default     any                `yaml:"default"`
	Nullable    bool               `yaml:"nullable"`
}

// Load reads and validates the OpenAPI document.
func Load(r io.Reader) (*Document, error) {
	var doc Document
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI document: %w", err)
	}
	if len(doc.Paths) == 0 {
		return nil, fmt.Errorf("OpenAPI document contains no paths")
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = map[string]*Schema{}
	}
	return &doc, nil
}

// LoadFS reads the document from an fs.FS.
func LoadFS(fsys fs.FS, name string) (*Document, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", name, err)
	}
	defer f.Close()
	return Load(f)
}

// Resolve follows $ref chains and returns the concrete schema. It never
// returns nil for a non-nil input; an unresolvable ref is returned unchanged
// so callers can decide whether that is fatal.
func (d *Document) Resolve(s *Schema) *Schema {
	seen := map[string]bool{}
	for s != nil && s.Ref != "" {
		if seen[s.Ref] {
			return s
		}
		seen[s.Ref] = true
		name := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		next, ok := d.Components.Schemas[name]
		if !ok {
			return s
		}
		s = next
	}
	return s
}

// BodySchema returns the resolved JSON body schema for an operation.
func (d *Document) BodySchema(op *Operation) (*Schema, string) {
	if op.RequestBody == nil {
		return nil, ""
	}
	mt, ok := op.RequestBody.Content["application/json"]
	if !ok || mt.Schema == nil {
		return nil, ""
	}
	ref := mt.Schema.Ref
	return d.Resolve(mt.Schema), strings.TrimPrefix(ref, "#/components/schemas/")
}

// Methods lists the HTTP methods this CLI understands.
var Methods = []string{"get", "post", "put", "delete", "patch"}

// Operations returns all HTTP operations in a deterministic order.
func (d *Document) Operations() []OperationRef {
	var out []OperationRef
	for path, methods := range d.Paths {
		for _, method := range Methods {
			op, ok := methods[method]
			if !ok || op == nil {
				continue
			}
			out = append(out, OperationRef{Method: strings.ToUpper(method), Path: path, Op: op})
		}
	}
	return out
}

// OperationRef is an operation plus its HTTP coordinates.
type OperationRef struct {
	Method string
	Path   string
	Op     *Operation
}

// Key is the stable identifier used by overlay.yaml.
func (r OperationRef) Key() string { return r.Method + " " + r.Path }
