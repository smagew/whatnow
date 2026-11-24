package main

// JSON that keeps the order of keys, for editing a file someone else owns (~/.claude/settings.json): every other
// setting stays where it was. The output is formatted the way Python's json.dumps(indent=2, ensure_ascii=False)
// does it, so files written by the old Python installer do not change on the first run.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type kv struct {
	k string
	v any
}

type omap struct {
	keys []string
	vals map[string]any
}

func (m *omap) get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

func (m *omap) set(k string, v any) {
	if m.vals == nil {
		m.vals = map[string]any{}
	}
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

func (m *omap) del(k string) {
	if _, ok := m.vals[k]; !ok {
		return
	}
	delete(m.vals, k)
	for i, x := range m.keys {
		if x == k {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

func decodeOrdered(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, err := decodeValue(d)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err == nil {
		return nil, fmt.Errorf("extra data after the JSON value")
	}
	return v, nil
}

func decodeValue(d *json.Decoder) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			m := &omap{vals: map[string]any{}}
			for d.More() {
				kt, err := d.Token()
				if err != nil {
					return nil, err
				}
				v, err := decodeValue(d)
				if err != nil {
					return nil, err
				}
				m.set(kt.(string), v)
			}
			_, err := d.Token()
			return m, err
		case '[':
			l := []any{}
			for d.More() {
				v, err := decodeValue(d)
				if err != nil {
					return nil, err
				}
				l = append(l, v)
			}
			_, err := d.Token()
			return l, err
		}
		return nil, fmt.Errorf("unexpected %v", x)
	}
	return t, nil
}

func pyString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// encodeOrdered writes v; indent "" gives one line (used only to compare values).
func encodeOrdered(v any, indent string) []byte {
	var b strings.Builder
	writeOrdered(&b, v, indent, 0)
	return []byte(b.String())
}

func writeOrdered(b *strings.Builder, v any, indent string, depth int) {
	nl := func(d int) {
		if indent != "" {
			b.WriteString("\n" + strings.Repeat(indent, d))
		}
	}
	sep := ", "
	if indent != "" {
		sep = ","
	}
	switch x := v.(type) {
	case *omap:
		if len(x.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{")
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(sep)
			}
			nl(depth + 1)
			b.WriteString(pyString(k) + ": ")
			writeOrdered(b, x.vals[k], indent, depth+1)
		}
		nl(depth)
		b.WriteString("}")
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[")
		for i, e := range x {
			if i > 0 {
				b.WriteString(sep)
			}
			nl(depth + 1)
			writeOrdered(b, e, indent, depth+1)
		}
		nl(depth)
		b.WriteString("]")
	case string:
		b.WriteString(pyString(x))
	case json.Number:
		b.WriteString(string(x))
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case nil:
		b.WriteString("null")
	default:
		b.WriteString(fmt.Sprint(x))
	}
}
