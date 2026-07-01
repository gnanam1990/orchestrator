package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifests writes the given name->content files into a fresh temp dir
// and returns its path.
func writeManifests(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return dir
}

const validDelegate = `name: claude-code-cli
type: delegate
adapter: compliant
invoke: 'claude --bare -p "{task}"'
description: Delegate a coding task.
permission: ask
`

const validKnowledge = `name: house-style
type: knowledge
description: The team coding conventions.
`

// TestLoadDir_Valid loads a directory of well-formed manifests and checks the
// parsed fields.
func TestLoadDir_Valid(t *testing.T) {
	dir := writeManifests(t, map[string]string{
		"delegate.yaml": validDelegate,
		"knowledge.yml": validKnowledge, // also exercises the .yml extension
	})

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d manifests, want 2", len(got))
	}

	// Entries are ordered by filename: delegate.yaml, knowledge.yml.
	d := got[0]
	if d.Name != "claude-code-cli" || d.Type != TypeDelegate ||
		d.Adapter != AdapterCompliant || d.Permission != PermissionAsk ||
		d.Invoke == "" || d.Description == "" {
		t.Errorf("delegate manifest parsed incorrectly: %+v", d)
	}

	k := got[1]
	if k.Name != "house-style" || k.Type != TypeKnowledge {
		t.Errorf("knowledge manifest parsed incorrectly: %+v", k)
	}
	if k.Adapter != "" || k.Invoke != "" || k.Permission != "" {
		t.Errorf("knowledge manifest should have no delegate fields: %+v", k)
	}
}

// TestLoadDir_MissingRequiredField rejects a delegate manifest that omits a
// required field, and names that field in the error.
func TestLoadDir_MissingRequiredField(t *testing.T) {
	// Missing `permission`, which is required for delegate entries.
	const missingPermission = `name: no-perm
type: delegate
adapter: compliant
invoke: run-me
description: Missing its permission field.
`
	dir := writeManifests(t, map[string]string{"bad.yaml": missingPermission})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for missing required field, got nil")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("error should name the missing field %q; got: %v", "permission", err)
	}
	if !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("error should name the offending file; got: %v", err)
	}
}

// TestLoadDir_MissingDescription rejects a manifest with no description
// (required for every entry type).
func TestLoadDir_MissingDescription(t *testing.T) {
	const noDesc = `name: no-desc
type: knowledge
`
	dir := writeManifests(t, map[string]string{"nodesc.yaml": noDesc})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for missing description, got nil")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error should name the %q field; got: %v", "description", err)
	}
}

// TestLoadDir_InvalidEnum rejects a manifest whose permission is outside the
// allowed set.
func TestLoadDir_InvalidEnum(t *testing.T) {
	const badEnum = `name: bad-enum
type: delegate
adapter: compliant
invoke: run-me
description: Has an out-of-range permission.
permission: maybe
`
	dir := writeManifests(t, map[string]string{"enum.yaml": badEnum})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for invalid enum value, got nil")
	}
	if !strings.Contains(err.Error(), "permission") || !strings.Contains(err.Error(), "maybe") {
		t.Errorf("error should name the field and the bad value; got: %v", err)
	}
}

// TestLoadDir_InvalidTypeEnum rejects an unknown `type` value.
func TestLoadDir_InvalidTypeEnum(t *testing.T) {
	const badType = `name: bad-type
type: gadget
description: Not a knowledge or delegate entry.
`
	dir := writeManifests(t, map[string]string{"type.yaml": badType})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for invalid type value, got nil")
	}
	if !strings.Contains(err.Error(), "type") || !strings.Contains(err.Error(), "gadget") {
		t.Errorf("error should name the field and bad value; got: %v", err)
	}
}

// TestLoadDir_DuplicateName rejects two entries that share a name, and names
// both files involved.
func TestLoadDir_DuplicateName(t *testing.T) {
	dir := writeManifests(t, map[string]string{
		"a.yaml": validDelegate,
		"b.yaml": validDelegate, // same name: claude-code-cli
	})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "claude-code-cli") {
		t.Errorf("error should report the duplicate name; got: %v", err)
	}
	if !strings.Contains(err.Error(), "a.yaml") || !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("error should name both files; got: %v", err)
	}
}

// TestLoadDir_UnknownField rejects a manifest with a misspelled/unknown key so
// typos surface instead of silently becoming empty required fields.
func TestLoadDir_UnknownField(t *testing.T) {
	const typo = `name: typo
type: delegate
adapter: compliant
invoke: run-me
description: Has a misspelled permission key.
permision: ask
`
	dir := writeManifests(t, map[string]string{"typo.yaml": typo})

	if _, err := LoadDir(dir); err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

// TestLoadDir_MultiDocumentRejected rejects a file holding more than one YAML
// document, so a second entry cannot be silently dropped past validation and
// duplicate-name detection.
func TestLoadDir_MultiDocumentRejected(t *testing.T) {
	// A valid first document followed by a second delegate document missing its
	// required fields — the exact silent-drop the guard closes.
	const multiDoc = validKnowledge + "---\n" + `name: sneaky
type: delegate
adapter: compliant
description: Missing invoke and permission.
`
	dir := writeManifests(t, map[string]string{"multi.yaml": multiDoc})

	_, err := LoadDir(dir)
	if err == nil {
		t.Fatal("expected error for multi-document file, got nil")
	}
	if !strings.Contains(err.Error(), "more than one YAML document") || !strings.Contains(err.Error(), "multi.yaml") {
		t.Errorf("error should name the file and the multi-document cause; got: %v", err)
	}
}

// TestLoadDir_TrailingSeparatorAllowed confirms the multi-document guard does
// not false-positive on a single document that ends with a bare "---".
func TestLoadDir_TrailingSeparatorAllowed(t *testing.T) {
	dir := writeManifests(t, map[string]string{"trailing.yaml": validKnowledge + "---\n"})

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("trailing separator should be allowed; got error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "house-style" {
		t.Fatalf("expected the single house-style entry; got %+v", got)
	}
}

// TestLoadDir_IgnoresNonYAML confirms non-manifest files are skipped.
func TestLoadDir_IgnoresNonYAML(t *testing.T) {
	dir := writeManifests(t, map[string]string{
		"delegate.yaml": validDelegate,
		"README.md":     "# not a manifest",
		"notes.txt":     "ignore me",
	})

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d manifests, want 1 (non-YAML files should be ignored)", len(got))
	}
}

// TestLoadDir_Seeds ensures the seed manifests shipped in ../manifests parse
// and validate.
func TestLoadDir_Seeds(t *testing.T) {
	got, err := LoadDir(filepath.Join("..", "manifests"))
	if err != nil {
		t.Fatalf("loading seed manifests: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d seed manifests, want 2", len(got))
	}
	byName := map[string]Manifest{}
	for _, m := range got {
		byName[m.Name] = m
	}
	if _, ok := byName["claude-code-cli"]; !ok {
		t.Errorf("expected a claude-code-cli seed manifest; got %v", byName)
	}
	if _, ok := byName["weather"]; !ok {
		t.Errorf("expected a weather seed manifest; got %v", byName)
	}
}
