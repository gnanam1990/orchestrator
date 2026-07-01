// Package catalog defines the manifest schema for orchestrator entries and a
// loader that reads, parses, and validates manifests from a directory.
package catalog

import "fmt"

// Type is the kind of catalog entry.
type Type string

const (
	// TypeKnowledge is a passive, informational entry (no invocation).
	TypeKnowledge Type = "knowledge"
	// TypeDelegate is an entry the orchestrator can invoke via an adapter.
	TypeDelegate Type = "delegate"
)

// Adapter identifies how a delegate entry is invoked.
type Adapter string

const (
	AdapterCompliant     Adapter = "compliant"
	AdapterComputerUse   Adapter = "computeruse"
	AdapterSandboxedRepo Adapter = "sandboxedrepo"
)

// Permission controls whether a delegate may run without user confirmation.
type Permission string

const (
	PermissionAuto  Permission = "auto"
	PermissionAsk   Permission = "ask"
	PermissionNever Permission = "never"
)

// Manifest is a single catalog entry, matching the YAML schema.
type Manifest struct {
	Name        string     `yaml:"name"`
	Type        Type       `yaml:"type"`
	Adapter     Adapter    `yaml:"adapter,omitempty"`
	Invoke      string     `yaml:"invoke,omitempty"`
	Description string     `yaml:"description"`
	Permission  Permission `yaml:"permission,omitempty"`
}

// validTypes, validAdapters, and validPermissions back enum validation.
var (
	validTypes = map[Type]bool{
		TypeKnowledge: true,
		TypeDelegate:  true,
	}
	validAdapters = map[Adapter]bool{
		AdapterCompliant:     true,
		AdapterComputerUse:   true,
		AdapterSandboxedRepo: true,
	}
	validPermissions = map[Permission]bool{
		PermissionAuto:  true,
		PermissionAsk:   true,
		PermissionNever: true,
	}
)

// Validate checks a single manifest against the schema. It reports the first
// field that fails so callers can produce a clear, actionable error.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("field %q is required", "name")
	}
	if m.Type == "" {
		return fmt.Errorf("field %q is required", "type")
	}
	if !validTypes[m.Type] {
		return fmt.Errorf("field %q has invalid value %q (want one of: knowledge, delegate)", "type", m.Type)
	}
	if m.Description == "" {
		return fmt.Errorf("field %q is required", "description")
	}

	// Fields required only for delegate entries.
	if m.Type == TypeDelegate {
		if m.Adapter == "" {
			return fmt.Errorf("field %q is required when type is %q", "adapter", TypeDelegate)
		}
		if !validAdapters[m.Adapter] {
			return fmt.Errorf("field %q has invalid value %q (want one of: compliant, computeruse, sandboxedrepo)", "adapter", m.Adapter)
		}
		if m.Invoke == "" {
			return fmt.Errorf("field %q is required when type is %q", "invoke", TypeDelegate)
		}
		if m.Permission == "" {
			return fmt.Errorf("field %q is required when type is %q", "permission", TypeDelegate)
		}
		if !validPermissions[m.Permission] {
			return fmt.Errorf("field %q has invalid value %q (want one of: auto, ask, never)", "permission", m.Permission)
		}
	} else {
		// For non-delegate entries, delegate-only fields must not be set to
		// invalid enum values if present.
		if m.Adapter != "" && !validAdapters[m.Adapter] {
			return fmt.Errorf("field %q has invalid value %q (want one of: compliant, computeruse, sandboxedrepo)", "adapter", m.Adapter)
		}
		if m.Permission != "" && !validPermissions[m.Permission] {
			return fmt.Errorf("field %q has invalid value %q (want one of: auto, ask, never)", "permission", m.Permission)
		}
	}

	return nil
}
