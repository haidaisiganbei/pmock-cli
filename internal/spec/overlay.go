package spec

import (
	"fmt"
	"io"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// Overlay supplies the product decisions that the OpenAPI document cannot
// express: the user-facing command path, aliases, risk level, and whether an
// operation is an internal building block that should stay out of `--help`.
type Overlay struct {
	Commands []CommandOverride `yaml:"commands"`
}

// CommandOverride overrides one operation. Op is "METHOD /path".
type CommandOverride struct {
	Op          string   `yaml:"op"`
	Path        []string `yaml:"path"`
	Aliases     []string `yaml:"aliases,omitempty"`
	Risk        string   `yaml:"risk,omitempty"`
	Internal    bool     `yaml:"internal,omitempty"`
	Async       string   `yaml:"async,omitempty"`
	OperateType string   `yaml:"operate_type,omitempty"`
	Examples    []string `yaml:"examples,omitempty"`
}

// LoadOverlay parses overlay.yaml.
func LoadOverlay(r io.Reader) (*Overlay, error) {
	var o Overlay
	if err := yaml.NewDecoder(r).Decode(&o); err != nil {
		return nil, fmt.Errorf("failed to parse overlay: %w", err)
	}
	return &o, nil
}

// LoadOverlayFS parses overlay.yaml from an fs.FS.
func LoadOverlayFS(fsys fs.FS, name string) (*Overlay, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", name, err)
	}
	defer f.Close()
	return LoadOverlay(f)
}

func (o *Overlay) find(key string) *CommandOverride {
	if o == nil {
		return nil
	}
	for i := range o.Commands {
		if o.Commands[i].Op == key {
			return &o.Commands[i]
		}
	}
	return nil
}
