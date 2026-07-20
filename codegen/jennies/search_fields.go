package jennies

import (
	"fmt"
	"slices"
	"strings"

	"cuelang.org/go/cue"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/searchfields"
)

// validateSearchFields checks every search field declared on a kind version:
// capabilities must be compatible with the declared field type (see the
// searchfields package), and each non-empty path must resolve against the
// version schema to a field whose type is compatible with the declared type.
func validateSearchFields(vk codegen.VersionedKind, version string) error {
	for _, sf := range vk.SearchFields {
		if err := validateSearchFieldCapabilities(sf); err != nil {
			return fmt.Errorf("kind %q version %q search field %q: %w", vk.Kind, version, sf.Name, err)
		}
		if sf.Path == "" || !vk.Schema.Exists() {
			continue
		}
		leaf, resolved, err := resolveSearchFieldPath(vk.Schema, sf.Path)
		if err != nil {
			return fmt.Errorf("kind %q version %q search field %q: %w", vk.Kind, version, sf.Name, err)
		}
		// resolved is false (with no error) when the path crosses a CUE
		// disjunction; the type cannot be checked against a single variant, so
		// the field is left as-is.
		if resolved {
			warning, err := searchFieldTypeCompatibility(leaf, sf.Type)
			if err != nil {
				return fmt.Errorf("kind %q version %q search field %q: %w", vk.Kind, version, sf.Name, err)
			}
			if warning != "" {
				//nolint:revive // release-please: ignore
				fmt.Printf("[WARN] kind %q version %q search field %q: %s\n", vk.Kind, version, sf.Name, warning)
			}
		}
	}
	return nil
}

// validateSearchFieldCapabilities delegates to the shared searchfields matrix,
// the single source of truth for which capabilities are valid on which field
// types. Grafana's runtime validator consumes the same matrix.
func validateSearchFieldCapabilities(sf codegen.SearchField) error {
	return searchfields.Validate(sf.Type, sf.Capabilities)
}

type pathSegment struct {
	name       string
	projection bool
}

func parseSearchFieldPath(path string) ([]pathSegment, error) {
	parts := strings.Split(path, ".")
	segs := make([]pathSegment, len(parts))
	for i, p := range parts {
		projection := strings.HasSuffix(p, "[*]")
		name := strings.TrimSuffix(p, "[*]")
		if name == "" {
			return nil, fmt.Errorf("invalid path %q", path)
		}
		segs[i] = pathSegment{name: name, projection: projection}
	}
	return segs, nil
}

// resolveSearchFieldPath walks a dot-separated path (with optional "[*]" array
// projection segments) against the schema. It returns the resolved leaf value
// and resolved=true when every segment resolves to a concrete field. It returns
// resolved=false with a nil error when the path descends into a CUE disjunction,
// since the leaf type cannot be pinned to a single variant. It returns an error
// when the path does not resolve (in any variant) or uses "[*]" on a non-list.
func resolveSearchFieldPath(schema cue.Value, path string) (cue.Value, bool, error) {
	segs, err := parseSearchFieldPath(path)
	if err != nil {
		return cue.Value{}, false, err
	}
	leaf, resolved, err := resolveSegments(schema, segs)
	if err != nil {
		return cue.Value{}, false, fmt.Errorf("path %q does not resolve: %w", path, err)
	}
	return leaf, resolved, nil
}

// resolveSegments walks segs against v. When a segment is not a direct field of
// v and v is a disjunction, it resolves the remaining path against each non-null
// variant: the path is accepted (resolved=false, because the leaf type spans
// variants) only if it fully resolves in at least one variant, and rejected
// when it resolves in none. This validates the entire path, including segments
// nested inside a variant, rather than stopping at the first variant match.
func resolveSegments(v cue.Value, segs []pathSegment) (cue.Value, bool, error) {
	for i, seg := range segs {
		next, ok := lookupSchemaField(v, seg.name)
		if !ok {
			if variants, _ := disjunctionVariants(v); len(variants) > 0 {
				remaining := segs[i:]
				for _, variant := range variants {
					if _, _, err := resolveSegments(variant, remaining); err == nil {
						return cue.Value{}, false, nil
					}
				}
				return cue.Value{}, false, fmt.Errorf("field %q not found in any schema variant", seg.name)
			}
			return cue.Value{}, false, fmt.Errorf("field %q not found", seg.name)
		}
		v = next

		if seg.projection {
			elem := v.LookupPath(cue.MakePath(cue.AnyIndex))
			if !elem.Exists() {
				return cue.Value{}, false, fmt.Errorf("field %q is not a list but is used with [*]", seg.name)
			}
			v = elem
		}
	}
	return v, true, nil
}

// searchFieldTypeCompatibility classifies a declared search field type against
// the schema type at the resolved leaf. It returns an error for a clear
// contradiction (for example a boolean type declared on a string field) and a
// non-empty warning for a compatible-but-lossy declaration (a fractional schema
// field indexed as int64, which the extractor rounds rather than drops).
//
// The accepted sets mirror what the consuming extractor can coerce at runtime,
// so a declaration that passes here is never silently dropped during indexing.
// The only consumer today is Grafana's scalar coercion in its search path
// extractor. This mapping is duplicated by convention across the two
// repositories; a future change could move extraction into generated code or
// this library so the two cannot drift. The rule of thumb is that this validator may be more
// lenient than a given consumer, but must not accept a type the consumer
// cannot extract. Kinds it cannot classify (structs other than time.Time, or
// anything unresolved) are skipped rather than flagged.
func searchFieldTypeCompatibility(leaf cue.Value, declared string) (warning string, err error) {
	// A list-typed leaf (a field that is itself an array, declared with
	// array: true and no "[*]" projection) is checked against its element type.
	if leaf.IncompleteKind() == cue.ListKind {
		if elem := leaf.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
			leaf = elem
		}
	}

	compatible := searchTypesForSchema(leaf)
	if compatible == nil {
		return "", nil
	}
	if !slices.Contains(compatible, declared) {
		return "", fmt.Errorf("declared type %q is not compatible with schema type %s", declared, leaf.IncompleteKind())
	}
	if declared == "int64" {
		switch leaf.IncompleteKind() {
		case cue.NumberKind, cue.FloatKind:
			return fmt.Sprintf("schema type %s indexed as int64 will be rounded at extraction", leaf.IncompleteKind()), nil
		}
	}
	return "", nil
}

// searchTypesForSchema returns the search field types a schema value can back,
// or nil when the value's kind cannot be classified (and so should not be
// checked). The sets mirror the extractor's runtime coercion: a string can back
// a date (it is kept verbatim), and either numeric kind can back int64 or
// double (the extractor rounds or widens as needed). time.Time resolves to
// StringKind, so timestamps fall into the string set.
func searchTypesForSchema(v cue.Value) []string {
	switch v.IncompleteKind() {
	case cue.StringKind:
		return []string{"string", "date"}
	case cue.IntKind, cue.NumberKind, cue.FloatKind:
		return []string{"int64", "double"}
	case cue.BoolKind:
		return []string{"boolean"}
	}
	return nil
}

// lookupSchemaField resolves a single field name against a schema value,
// trying both the required and optional forms.
func lookupSchemaField(v cue.Value, name string) (cue.Value, bool) {
	p := cue.MakePath(cue.Str(name))
	if l := v.LookupPath(p); l.Exists() {
		return l, true
	}
	if l := v.LookupPath(p.Optional()); l.Exists() {
		return l, true
	}
	return cue.Value{}, false
}
