package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MaxRepetitions caps one experiment submission so a single transaction
// cannot insert an unbounded number of trials.
const MaxRepetitions = 30

// UnmarshalStrict rejects duplicate keys, unknown fields, extra documents,
// and payloads over 1 MiB. Publish uses it so a draft cannot drop fields quietly.
func UnmarshalStrict(data []byte, dest any) error {
	if len(data) > 1<<20 {
		return fmt.Errorf("draft exceeds 1 MiB")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("draft must contain one document")
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return walkJSON(dec, 0)
}

func walkJSON(dec *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("draft is too deep")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]struct{}{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, dup := seen[key]; dup {
				return fmt.Errorf("duplicate key %s", key)
			}
			seen[key] = struct{}{}
			if err := walkJSON(dec, depth+1); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case json.Delim('['):
		for dec.More() {
			if err := walkJSON(dec, depth+1); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	default:
		return nil
	}
}
