package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// Canonical JSON sorts object keys and rejects floats so money cannot sneak in
// as a binary fraction. Arrays keep their order.
func Canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var parsed any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, parsed); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Digest(v any) (string, error) {
	b, err := Canonical(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	case json.Number:
		if _, err := t.Int64(); err != nil {
			return fmt.Errorf("non-integer number %s", t)
		}
		buf.WriteString(t.String())
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical type %T", v)
	}
	return nil
}
