package spec

import (
	"fmt"
	"sort"
	"strings"
)

// Risk drives confirmation and warning behaviour.
type Risk string

const (
	RiskRead        Risk = "read"
	RiskWrite       Risk = "write"
	RiskCostly      Risk = "costly"
	RiskDestructive Risk = "destructive"
)

// Flag types understood by the command builder and the request encoder.
const (
	TypeString      = "string"
	TypeInteger     = "integer"
	TypeNumber      = "number"
	TypeBoolean     = "boolean"
	TypeStringArray = "string[]"
	TypeIntArray    = "integer[]"
)

// Flag is one generated command-line flag.
type Flag struct {
	Name        string // kebab-case CLI name, without --
	Field       string // JSON / query field name
	In          string // query, path, or body
	Type        string
	Description string
	Required    bool
	Enum        []any
	Default     any
}

// Help renders the flag description shown by --help.
func (f *Flag) Help() string {
	parts := []string{}
	desc := strings.Join(strings.Fields(f.Description), " ")
	// pflag treats a backquoted word as the flag's value placeholder. The
	// spec uses backticks for field references, which would turn
	// "--authorize-user-ids" into "--authorize-user-ids userId".
	desc = strings.ReplaceAll(desc, "`", "'")
	if desc != "" {
		parts = append(parts, desc)
	}
	if f.Type == TypeStringArray || f.Type == TypeIntArray {
		parts = append(parts, "comma-separated")
	}
	if len(f.Enum) > 0 {
		vals := make([]string, 0, len(f.Enum))
		for _, v := range f.Enum {
			vals = append(vals, fmt.Sprint(v))
		}
		parts = append(parts, "allowed: "+strings.Join(vals, ", "))
	}
	if f.Default != nil {
		parts = append(parts, fmt.Sprintf("default: %v", f.Default))
	}
	if f.Required {
		parts = append(parts, "required")
	}
	return strings.Join(parts, "; ")
}

// Command is one generated CLI leaf command.
type Command struct {
	Key         string // METHOD /path
	Method      string
	Path        string
	Summary     string
	Description string

	PathParts   []string
	Aliases     []string
	Risk        Risk
	Internal    bool
	Async       string
	OperateType string
	Examples    []string

	Flags []*Flag
	// ComplexRequired lists required body fields that cannot be a flag
	// (arrays of objects / nested objects); they must be supplied via --json.
	ComplexRequired []string

	Body           *Schema
	BodySchemaName string
}

// Usage is the last command-path segment.
func (c *Command) Usage() string {
	if len(c.PathParts) == 0 {
		return ""
	}
	return c.PathParts[len(c.PathParts)-1]
}

// FullPath is the space-joined command path.
func (c *Command) FullPath() string { return strings.Join(c.PathParts, " ") }

// LongHelp is the description shown by `<command> --help`.
func (c *Command) LongHelp() string {
	var b strings.Builder
	desc := strings.TrimSpace(c.Description)
	if desc == "" {
		desc = strings.TrimSpace(c.Summary)
	}
	b.WriteString(desc)
	b.WriteString("\n\nHTTP: ")
	b.WriteString(c.Method)
	b.WriteString(" ")
	b.WriteString(c.Path)
	b.WriteString("\nRisk: ")
	b.WriteString(string(c.Risk))
	if c.Risk == RiskCostly {
		b.WriteString(" (this call can spend money)")
	}
	if c.Risk == RiskDestructive {
		b.WriteString(" (irreversible; requires --yes)")
	}
	if c.HasJSONInput() {
		b.WriteString("\nBody: pass individual flags, or --json '<object>' / --json @file.json to supply nested fields")
	}
	if c.CanWait() {
		b.WriteString("\nWait: --wait polls the task to a terminal state (--wait-timeout, --poll-interval)")
	}
	if len(c.ComplexRequired) > 0 {
		b.WriteString("\nRequired nested body fields (use --json): ")
		b.WriteString(strings.Join(c.ComplexRequired, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// HasJSONInput reports whether --json is meaningful for this command.
func (c *Command) HasJSONInput() bool { return c.Body != nil }

// HasBody reports whether the operation sends a JSON body.
func (c *Command) HasBody() bool { return c.Body != nil }

// CanWait reports whether --wait is available for this command.
func (c *Command) CanWait() bool { return c.Async != "" }

// Catalog is the full command surface.
type Catalog struct {
	Commands []*Command
}

// BuildCatalog merges the OpenAPI operations with the overlay and derives
// flags. The result is deterministic: commands are sorted by their full path.
func BuildCatalog(doc *Document, overlay *Overlay) (*Catalog, error) {
	var cmds []*Command
	seenPath := map[string]*Command{}
	seenAlias := map[string]string{}

	for _, ref := range doc.Operations() {
		ov := overlay.find(ref.Key())
		cmd, err := buildCommand(doc, ref, ov)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ref.Key(), err)
		}
		full := cmd.FullPath()
		if prev, ok := seenPath[full]; ok {
			return nil, fmt.Errorf("command path %q is used by both %s and %s", full, prev.Key, cmd.Key)
		}
		seenPath[full] = cmd
		for _, a := range cmd.Aliases {
			alias := strings.Join(append(append([]string{}, cmd.PathParts[:len(cmd.PathParts)-1]...), a), " ")
			if other, ok := seenAlias[alias]; ok {
				return nil, fmt.Errorf("alias %q for %s is already used by %s", alias, cmd.Key, other)
			}
			seenAlias[alias] = cmd.Key
		}
		cmds = append(cmds, cmd)
	}

	sort.Slice(cmds, func(i, j int) bool { return cmds[i].FullPath() < cmds[j].FullPath() })
	return &Catalog{Commands: cmds}, nil
}

func buildCommand(doc *Document, ref OperationRef, ov *CommandOverride) (*Command, error) {
	pathParts := derivePath(ref)
	risk := defaultRisk(ref.Method)
	var aliases, examples []string
	internal := false
	async := ""
	operateType := ""
	if ov != nil {
		if len(ov.Path) > 0 {
			pathParts = append([]string{}, ov.Path...)
		}
		aliases = append([]string{}, ov.Aliases...)
		examples = append([]string{}, ov.Examples...)
		internal = ov.Internal
		async = ov.Async
		operateType = ov.OperateType
		if ov.Risk != "" {
			risk = Risk(ov.Risk)
		}
	}
	if async != "" && async != "phone" && async != "action" {
		return nil, fmt.Errorf("invalid async %q", async)
	}
	if async == "phone" && operateType == "" {
		return nil, fmt.Errorf("async phone requires operate_type")
	}
	if len(pathParts) == 0 {
		return nil, fmt.Errorf("empty command path")
	}
	switch risk {
	case RiskRead, RiskWrite, RiskCostly, RiskDestructive:
	default:
		return nil, fmt.Errorf("invalid risk %q", risk)
	}

	body, bodyName := doc.BodySchema(ref.Op)
	flags, complexRequired, err := buildFlags(doc, ref.Op, body)
	if err != nil {
		return nil, err
	}

	return &Command{
		Key:             ref.Key(),
		Method:          ref.Method,
		Path:            ref.Path,
		Summary:         ref.Op.Summary,
		Description:     ref.Op.Description,
		PathParts:       pathParts,
		Aliases:         aliases,
		Risk:            risk,
		Internal:        internal,
		Async:           async,
		OperateType:     operateType,
		Examples:        examples,
		Flags:           flags,
		ComplexRequired: complexRequired,
		Body:            body,
		BodySchemaName:  bodyName,
	}, nil
}

func buildFlags(doc *Document, op *Operation, body *Schema) ([]*Flag, []string, error) {
	var flags []*Flag
	byName := map[string]*Flag{}

	add := func(f *Flag) error {
		if prev, ok := byName[f.Name]; ok {
			if prev.Field == f.Field {
				return nil
			}
			return fmt.Errorf("flag --%s maps to both %q and %q", f.Name, prev.Field, f.Field)
		}
		byName[f.Name] = f
		flags = append(flags, f)
		return nil
	}

	for i := range op.Parameters {
		p := &op.Parameters[i]
		if p.In == "header" || p.Name == "chameleon" {
			continue
		}
		sch := doc.Resolve(p.Schema)
		typ, ok := flagType(sch)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported parameter type for %s.%s", p.In, p.Name)
		}
		required := p.Required || p.In == "path"
		if err := add(&Flag{
			Name:        Kebab(p.Name),
			Field:       p.Name,
			In:          p.In,
			Type:        typ,
			Description: p.Description,
			Required:    required,
			Enum:        sch.Enum,
			Default:     sch.Default,
		}); err != nil {
			return nil, nil, err
		}
	}

	var complexRequired []string
	if body != nil && body.Type == "object" {
		keys := make([]string, 0, len(body.Properties))
		for k := range body.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		required := map[string]bool{}
		for _, r := range body.Required {
			required[r] = true
		}
		for _, field := range keys {
			prop := doc.Resolve(body.Properties[field])
			typ, ok := flagType(prop)
			if !ok {
				if required[field] {
					complexRequired = append(complexRequired, field)
				}
				continue
			}
			if err := add(&Flag{
				Name:        Kebab(field),
				Field:       field,
				In:          "body",
				Type:        typ,
				Description: prop.Description,
				Required:    required[field],
				Enum:        prop.Enum,
				Default:     prop.Default,
			}); err != nil {
				return nil, nil, err
			}
		}
	}

	sort.Slice(flags, func(i, j int) bool {
		if flags[i].Required != flags[j].Required {
			return flags[i].Required
		}
		return flags[i].Name < flags[j].Name
	})
	return flags, complexRequired, nil
}

func flagType(s *Schema) (string, bool) {
	if s == nil {
		return "", false
	}
	switch s.Type {
	case "string":
		return TypeString, true
	case "integer", "int", "int32", "int64":
		return TypeInteger, true
	case "number":
		return TypeNumber, true
	case "boolean":
		return TypeBoolean, true
	case "array":
		item := s.Items
		if item == nil {
			return "", false
		}
		switch item.Type {
		case "string":
			return TypeStringArray, true
		case "integer", "int", "int32", "int64":
			return TypeIntArray, true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func defaultRisk(method string) Risk {
	if strings.EqualFold(method, "GET") {
		return RiskRead
	}
	return RiskWrite
}

// Kebab converts camelCase / API field names to CLI flag names.
func Kebab(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r == '_' {
			b.WriteRune('-')
			continue
		}
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := runes[i-1]
				nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
				if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') || (prev >= 'A' && prev <= 'Z' && nextLower) {
					b.WriteRune('-')
				}
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// derivePath is the fallback when overlay.yaml has no entry. It strips the
// API prefix and normalises the handful of verbs the spec uses. Every current
// operation has an overlay entry, so this is a safety net for future additions.
func derivePath(ref OperationRef) []string {
	trimmed := strings.TrimPrefix(ref.Path, "/open/api/")
	trimmed = strings.Trim(trimmed, "/")
	segments := strings.Split(trimmed, "/")

	var out []string
	for _, seg := range segments {
		if strings.HasPrefix(seg, "{") {
			continue
		}
		out = append(out, normalizeSegment(seg))
	}

	switch {
	case len(out) == 1 && out[0] == "get-phone-model-list":
		return []string{"phone", "models"}
	case len(out) >= 2 && out[0] == "action":
		return append([]string{"phone"}, out...)
	case len(out) >= 1 && out[0] == "get-cpy-connect":
		return []string{"phone", "cpy-connect"}
	case len(out) >= 1 && out[0] == "get-remote-component-token":
		return []string{"phone", "stream-token"}
	case len(out) >= 2 && out[0] == "team" && out[1] == "member":
		return []string{"team", "members"}
	}
	return out
}

func normalizeSegment(seg string) string {
	switch seg {
	case "getList":
		return "list"
	case "getInfo":
		return "get"
	case "getMarkList":
		return "market"
	case "getDetailList":
		return "details"
	case "categoryList":
		return "categories"
	case "listUpgradeApp":
		return "upgrades"
	case "getAdbInfo":
		return "adb"
	case "oneNew":
		return "new"
	case "batchRenew":
		return "renew"
	case "screenCap":
		return "screen"
	case "getRemoteToken":
		return "remote-token"
	case "getCpyConnect":
		return "cpy-connect"
	case "getRemoteComponentToken":
		return "stream-token"
	case "setNetwork":
		return "network-set"
	case "completeAndPush":
		return "upload-push"
	case "init":
		return "upload-init"
	case "getPhoneModelList":
		return "get-phone-model-list"
	}
	return Kebab(seg)
}
