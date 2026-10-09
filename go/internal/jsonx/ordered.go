// Package jsonx emits the byte-stable JSON format used by Contract artifacts.
// It differs from encoding/json defaults in three ways:
//  1. map keys are sorted, not insertion-ordered;
//  2. `<`, `>`, `&` are HTML-escaped by default;
//  3. nil slices marshal as `null`, not `[]`.
//
// All byte-critical JSON (dist/agent-pack.manifest.json, .pi/settings.json,
// .claude/settings.json, workspace-profile.json) must go
// through this package, not json.Marshal directly.
package jsonx

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

// OrderedMap is a minimal insertion-ordered map. Keys are emitted in the order
// they were first Set.
type OrderedMap struct {
	keys []string
	vals map[string]any
}

func NewMap() *OrderedMap { return &OrderedMap{vals: map[string]any{}} }

func (m *OrderedMap) Set(key string, value any) *OrderedMap {
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = value
	return m
}

func (m *OrderedMap) Get(key string) (any, bool) { v, ok := m.vals[key]; return v, ok }
func (m *OrderedMap) Len() int                   { return len(m.keys) }

// Keys returns the keys in insertion order.
func (m *OrderedMap) Keys() []string { return m.keys }

// Quote returns s as a JSON string literal (surrounding quotes included) using
// the Contract artifact escaping rules.
func Quote(s string) string {
	var buf bytes.Buffer
	writeString(&buf, s)
	return buf.String()
}

// Plain recursively converts *OrderedMap to map[string]any (and []any stays
// []any), leaving scalars untouched. Used to feed encoding/json, yaml.v3 and
// pelletier, which don't understand *OrderedMap.
func Plain(v any) any {
	switch t := v.(type) {
	case *OrderedMap:
		out := make(map[string]any, t.Len())
		for _, k := range t.keys {
			val, _ := t.Get(k)
			out[k] = Plain(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = Plain(e)
		}
		return out
	default:
		return v
	}
}

// Encode writes v with the given indent and no trailing newline.
func Encode(v any, indent string) []byte {
	var buf bytes.Buffer
	writeValue(&buf, v, indent, 0)
	return buf.Bytes()
}

// Marshal writes v with the given indent and a trailing newline.
func Marshal(v any, indent string) ([]byte, error) {
	out := Encode(v, indent)
	out = append(out, '\n')
	return out, nil
}

func writeIndent(buf *bytes.Buffer, indent string, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString(indent)
	}
}

func writeValue(buf *bytes.Buffer, v any, indent string, depth int) {
	switch t := v.(type) {
	case *OrderedMap:
		writeObject(buf, t, indent, depth)
	case []any:
		writeArray(buf, t, indent, depth)
	case []string:
		arr := make([]any, len(t))
		for i, s := range t {
			arr[i] = s
		}
		writeArray(buf, arr, indent, depth)
	case map[string]any:
		// Plain maps lose insertion order. Byte-critical payloads must never
		// hit this path (convert TOML tables to *OrderedMap); keys are sorted
		// here only for determinism.
		writePlainMap(buf, t, indent, depth)
	case string:
		writeString(buf, t)
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case float64:
		buf.WriteString(formatFloat(t))
	case nil:
		buf.WriteString("null")
	default:
		// Valid-JSON fallback for unexpected types; should not occur in
		// byte-critical output.
		writeString(buf, fmt.Sprintf("%v", t))
	}
}

func writeObject(buf *bytes.Buffer, m *OrderedMap, indent string, depth int) {
	if m.Len() == 0 {
		buf.WriteString("{}")
		return
	}
	buf.WriteString("{\n")
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeIndent(buf, indent, depth+1)
		writeString(buf, key)
		buf.WriteString(": ")
		writeValue(buf, m.vals[key], indent, depth+1)
	}
	buf.WriteByte('\n')
	writeIndent(buf, indent, depth)
	buf.WriteByte('}')
}

func writeArray(buf *bytes.Buffer, arr []any, indent string, depth int) {
	if len(arr) == 0 {
		buf.WriteString("[]")
		return
	}
	buf.WriteString("[\n")
	for i, item := range arr {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeIndent(buf, indent, depth+1)
		writeValue(buf, item, indent, depth+1)
	}
	buf.WriteByte('\n')
	writeIndent(buf, indent, depth)
	buf.WriteByte(']')
}

func writePlainMap(buf *bytes.Buffer, m map[string]any, indent string, depth int) {
	if len(m) == 0 {
		buf.WriteString("{}")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteString("{\n")
	for i, k := range keys {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeIndent(buf, indent, depth+1)
		writeString(buf, k)
		buf.WriteString(": ")
		writeValue(buf, m[k], indent, depth+1)
	}
	buf.WriteByte('\n')
	writeIndent(buf, indent, depth)
	buf.WriteByte('}')
}

// writeString escapes `"`, `\`, and control chars (named escapes for
// \b \f \n \r \t, \u00xx for the rest). It does not HTML-escape or escape
// forward slashes, and it preserves raw UTF-8.
func writeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// formatFloat uses the shortest round-trippable representation. Floats do not
// currently appear in agent-pack.toml / provider files; revisit if they do.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
