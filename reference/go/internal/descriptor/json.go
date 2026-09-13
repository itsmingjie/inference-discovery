package descriptor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// CheckJSON rejects ambiguous duplicate keys, excess nesting, and trailing values.
func CheckJSON(b []byte) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("invalid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 16 {
			return fmt.Errorf("JSON nesting exceeds 16")
		}
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{':
				seen := map[string]bool{}
				for dec.More() {
					k, err := dec.Token()
					if err != nil {
						return err
					}
					s, ok := k.(string)
					if !ok || seen[s] {
						return fmt.Errorf("duplicate or invalid object key")
					}
					seen[s] = true
					if err = value(depth + 1); err != nil {
						return err
					}
				}
				t, err = dec.Token()
				if err != nil || t != json.Delim('}') {
					return fmt.Errorf("invalid object")
				}
			case '[':
				for dec.More() {
					if err = value(depth + 1); err != nil {
						return err
					}
				}
				t, err = dec.Token()
				if err != nil || t != json.Delim(']') {
					return fmt.Errorf("invalid array")
				}
			default:
				return fmt.Errorf("unexpected JSON delimiter")
			}
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}
