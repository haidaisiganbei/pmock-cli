// Package output renders the API envelope and human-readable variants.
//
// stdout always receives exactly one value: the envelope for --format json,
// or a rendering of it for pretty/table. Diagnostics belong on stderr.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/haidaisiganbei/pmock-cli/internal/client"
)

// Options controls rendering.
type Options struct {
	Format  string
	Compact bool
}

// Render writes env to w.
func Render(w io.Writer, env *client.Envelope, opts Options) error {
	if env == nil {
		return fmt.Errorf("empty response")
	}
	switch opts.Format {
	case "", "json":
		return renderJSON(w, env, opts.Compact)
	case "pretty":
		return renderPretty(w, env)
	case "table":
		return renderTable(w, env)
	default:
		return fmt.Errorf("unknown format %q (want json, pretty, table)", opts.Format)
	}
}

type envelopeJSON struct {
	TraceID string          `json:"traceId,omitempty"`
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
	Status  string          `json:"status,omitempty"`
}

func renderJSON(w io.Writer, env *client.Envelope, compact bool) error {
	data := env.Data
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	out := envelopeJSON{TraceID: env.TraceID, Code: env.Code, Msg: env.Msg, Data: data, Status: env.Status}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(out)
}

func renderPretty(w io.Writer, env *client.Envelope) error {
	if env.TraceID != "" {
		fmt.Fprintf(w, "TraceID: %s\n", env.TraceID)
	}
	if env.OK() {
		fmt.Fprintf(w, "Status:  OK (code=0, %s)\n", env.Msg)
	} else {
		fmt.Fprintf(w, "Status:  FAILED (code=%d, %s)\n", env.Code, env.Msg)
	}
	if env.Status != "" {
		fmt.Fprintf(w, "State:   %s\n", env.Status)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}

	var v any
	if err := json.Unmarshal(env.Data, &v); err != nil {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(string(env.Data)))
		return nil
	}
	b, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nData:\n  %s\n", b)
	return nil
}

func renderTable(w io.Writer, env *client.Envelope) error {
	rows, label := extractRows(env.Data)
	if len(rows) == 0 {
		return renderPretty(w, env)
	}
	if env.TraceID != "" {
		fmt.Fprintf(w, "TraceID: %s\n", env.TraceID)
	}
	fmt.Fprintf(w, "%s (%d rows)\n\n", label, len(rows))
	fmt.Fprint(w, renderRows(rows))
	return nil
}

func extractRows(raw json.RawMessage) ([]map[string]any, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, "data"
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, ""
	}
	for _, key := range []string{"rows", "list", "items", "data"} {
		v, ok := obj[key]
		if !ok {
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(b, &arr); err == nil {
			return arr, key
		}
	}
	return nil, ""
}

func renderRows(rows []map[string]any) string {
	// Union of keys, sorted for a stable layout across runs.
	keySet := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			keySet[k] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}

	widths := make(map[string]int, len(keys))
	for _, k := range keys {
		widths[k] = displayWidth(k)
	}
	cells := make([][]string, len(rows))
	for i, r := range rows {
		cells[i] = make([]string, len(keys))
		for j, k := range keys {
			s := formatCell(r[k])
			cells[i][j] = s
			if n := displayWidth(s); n > widths[k] {
				widths[k] = n
			}
		}
	}
	for _, k := range keys {
		if widths[k] > 40 {
			widths[k] = 40
		}
	}

	var b strings.Builder
	writeLine := func(vals []string, upper bool) {
		for i, k := range keys {
			if i > 0 {
				b.WriteString("  ")
			}
			s := vals[i]
			if upper {
				s = strings.ToUpper(s)
			}
			b.WriteString(pad(s, widths[k]))
		}
		b.WriteString("\n")
	}

	header := make([]string, len(keys))
	copy(header, keys)
	writeLine(header, true)
	for i := range keys {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("-", widths[keys[i]]))
	}
	b.WriteString("\n")

	vals := make([]string, len(keys))
	for _, row := range cells {
		copy(vals, row)
		writeLine(vals, false)
	}
	return b.String()
}

func formatCell(v any) string {
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case map[string]any, []any:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return truncate(string(b), 40)
	default:
		return truncate(fmt.Sprint(t), 40)
	}
}

func truncate(s string, max int) string {
	if displayWidth(s) <= max {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && displayWidth(string(runes)) > max-3 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

func pad(s string, width int) string {
	if n := displayWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// displayWidth counts terminal cells; CJK runes are double width. This keeps
// Chinese phone names aligned, unlike a byte or rune count.
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			width++
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hangul, r),
			unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			width += 2
		default:
			width++
		}
	}
	return width
}
