package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadDir reads every .yaml/.yml file from dir, parses and validates each
// manifest against the schema, and enforces that no two entries share a name.
// Entries are returned sorted by source filename for deterministic output.
//
// On failure it returns an error naming the offending file and field so the
// caller can point the user straight at the problem.
func LoadDir(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("catalog: reading directory %q: %w", dir, err)
	}

	// Collect manifest filenames deterministically so both loading order and
	// duplicate-detection reporting are stable across runs.
	var files []string
	for _, e := range entries {
		if e.IsDir() || !isManifestFile(e.Name()) {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	manifests := make([]Manifest, 0, len(files))
	seen := make(map[string]string, len(files)) // name -> filename that first declared it
	for _, name := range files {
		m, err := loadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if prev, ok := seen[m.Name]; ok {
			return nil, fmt.Errorf("catalog: %s: duplicate name %q (already declared in %s)", name, m.Name, prev)
		}
		seen[m.Name] = name
		manifests = append(manifests, *m)
	}
	return manifests, nil
}

// isManifestFile reports whether name has a YAML extension.
func isManifestFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

// loadFile reads, parses, and validates a single manifest file.
func loadFile(path string) (*Manifest, error) {
	base := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("catalog: reading %s: %w", base, err)
	}

	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // reject unknown/misspelled keys instead of silently dropping them
	switch err := dec.Decode(&m); {
	case errors.Is(err, io.EOF):
		// Empty document; fall through so Validate reports the specific required
		// field that is missing rather than a cryptic EOF.
	case err != nil:
		return nil, fmt.Errorf("catalog: %s: parse error: %w", base, err)
	default:
		// One document decoded. Guard against a silently-dropped second document:
		// a real second manifest (or malformed trailing content) would otherwise
		// vanish unvalidated and skip duplicate-name detection. A bare trailing
		// "---" decodes as an empty document and is harmless.
		var extra Manifest
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) && !(err == nil && extra == (Manifest{})) {
			return nil, fmt.Errorf("catalog: %s: contains more than one YAML document (one manifest per file)", base)
		}
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("catalog: %s: %w", base, err)
	}
	return &m, nil
}
