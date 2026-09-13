// Package cli maps loupe's commands onto cobra and turns domain results and refusals into the output contract.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const envelopeVersion = 1

var reservedKeys = map[string]bool{"loupe": true, "ok": true, "command": true, "run": true, "version": true, "error": true}

type field struct {
	key   string
	value any
}

// writeSuccess puts payload keys at the top level after the envelope keys, which come first so a human reading the
// object sees what it is before what it holds.
func writeSuccess(w io.Writer, command, run string, version *int, payload map[string]any) error {
	fields := []field{{"loupe", envelopeVersion}, {"ok", true}, {"command", command}}
	if run != "" {
		fields = append(fields, field{"run", run})
	}
	if version != nil {
		fields = append(fields, field{"version", *version})
	}
	keys := make([]string, 0, len(payload))
	for k := range payload {
		if reservedKeys[k] {
			return fmt.Errorf("payload key %q collides with the result envelope", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fields = append(fields, field{k, payload[k]})
	}
	return writeObject(w, fields)
}

func writeRefusal(w io.Writer, command, run string, r *refusal.Error) error {
	errFields := []field{{"code", string(r.Code)}, {"message", r.Message}, {"fix", r.Fix}}
	if len(r.Details) > 0 {
		errFields = append(errFields, field{"details", r.Details})
	}
	errObject, err := encodeObject(errFields)
	if err != nil {
		return err
	}
	fields := []field{{"loupe", envelopeVersion}, {"ok", false}, {"command", command}}
	if run != "" {
		fields = append(fields, field{"run", run})
	}
	fields = append(fields, field{"error", json.RawMessage(errObject)})
	return writeObject(w, fields)
}

func writeObject(w io.Writer, fields []field) error {
	data, err := encodeObject(fields)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func encodeObject(fields []field) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := encodeValue(f.key)
		if err != nil {
			return nil, err
		}
		value, err := encodeValue(f.value)
		if err != nil {
			return nil, fmt.Errorf("encode result field %q: %w", f.key, err)
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encodeValue leaves <, > and & unescaped because results carry Markdown that agents read back verbatim.
func encodeValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
