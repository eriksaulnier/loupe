package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// WriteFileAtomic leaves either the old or the new content at path, never a partial file, even across a crash.
func WriteFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmp.Name(), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp.Name(), path, err)
	}
	return syncDir(dir)
}

// syncDir makes a rename durable; without it the directory entry can be lost on power failure.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", dir, err)
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return d.Close()
}

func WriteJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return WriteFileAtomic(path, append(data, '\n'))
}

// ReadJSON reads a run file whose schema is from 1 up to schema. It refuses a damaged record rather than repairing it,
// so the human can inspect what went wrong, and refuses a newer one as a loupe too old to read it. It decodes every
// accepted schema into the current struct, so the first bump that removes or redefines a field MUST add a migration
// step here (docs/versioning.md).
func ReadJSON(path string, v any, schema int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return RecordRefusal(path, err)
	}
	// The schema is read first and leniently, because a newer file's new fields would otherwise refuse it as damaged.
	var head struct {
		Schema int `json:"schema"`
	}
	if json.Unmarshal(data, &head) == nil && head.Schema > schema {
		return refusal.New(refusal.Record,
			fmt.Sprintf("cannot read %s: schema is %d, this loupe reads up to %d", path, head.Schema, schema), "upgrade loupe")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return RecordRefusal(path, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return RecordRefusal(path, errors.New("unexpected data after the JSON value"))
	}
	if head.Schema < 1 {
		return RecordRefusal(path, fmt.Errorf("schema is %d, expected at least 1", head.Schema))
	}
	return nil
}

// RecordRefusal refuses a damaged run file, naming it and a command to inspect it; loupe never repairs one.
func RecordRefusal(path string, cause error) error {
	return refusal.New(refusal.Record, fmt.Sprintf("cannot read %s: %v", path, cause), "inspect it with: cat "+path)
}
