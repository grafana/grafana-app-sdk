//nolint:dupl
package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/resource"
)

type SchemaGenerator struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, the Kind() and Schema() functions are prefixed with the kind name,
	// i.e. FooKind() and FooSchema() for kind.Name()="Foo"
	GroupByKind bool
}

func (*SchemaGenerator) JennyName() string {
	return "SchemaGenerator"
}

// Generate creates one or more schema go files for the provided Kind
func (s *SchemaGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)
	for version, kind := range codegen.VersionedKinds(appManifest) {
		if kind.Scope != string(resource.NamespacedScope) && kind.Scope != string(resource.ClusterScope) {
			return nil, fmt.Errorf("%s/%s: scope '%s' is invalid, must be one of: '%s', '%s'",
				version.Name(), kind.Kind, kind.Scope, resource.ClusterScope, resource.NamespacedScope)
		}
		prefix := ""
		if !s.GroupByKind {
			prefix = exportField(kind.Kind)
		}
		sf, err := s.getSelectableFields(&kind)
		if err != nil {
			return nil, err
		}
		b := bytes.Buffer{}
		err = templates.WriteSchema(templates.SchemaMetadata{
			Package:          ToPackageName(version.Name()),
			Group:            appManifest.Properties().FullGroup,
			Version:          version.Name(),
			Kind:             kind.Kind,
			Plural:           kind.PluralMachineName,
			Scope:            kind.Scope,
			SelectableFields: sf,
			FuncPrefix:       prefix,
		}, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			Data:         formatted,
			RelativePath: filepath.Join(GetGeneratedGoTypePath(s.GroupByKind, appManifest.Properties().Group, version.Name(), kind.MachineName), fmt.Sprintf("%s_schema_gen.go", kind.MachineName)),
			From:         []codejen.NamedJenny{s},
		})
	}
	return files, nil
}

func (*SchemaGenerator) getSelectableFields(kind *codegen.VersionedKind) ([]templates.SchemaMetadataSelectableField, error) {
	fields := make([]templates.SchemaMetadataSelectableField, 0)
	if len(kind.SelectableFields) == 0 {
		return fields, nil
	}
	// Check each field in the CUE (TODO: make this OpenAPI instead?) to check if the field is optional
	for _, s := range kind.SelectableFields {
		fieldPath := s
		if len(s) > 1 && s[0] == '.' {
			fieldPath = s[1:]
		}
		parts := strings.Split(fieldPath, ".")
		if len(parts) <= 1 {
			return nil, fmt.Errorf("invalid selectable field path: %s", s)
		}
		field := parts[len(parts)-1]
		parts = parts[:len(parts)-1]
		path := make([]cue.Selector, 0)
		for _, p := range parts {
			path = append(path, cue.Str(p))
		}
		val := kind.Schema.LookupPath(cue.MakePath(path...).Optional())
		valErr := val.Err()
		if valErr == nil {
			cuePath := cue.MakePath(cue.Str(field))
			lookup, optional := lookupInValue(val, cuePath)
			if !lookup.Exists() {
				// The value may be a disjunction where the field only exists in some variants;
				// if so the field is implicitly optional.
				if lookup = lookupInDisjunction(val, cuePath, field); lookup.Exists() {
					optional = true
				}
			}

			if !lookup.Exists() {
				return nil, fmt.Errorf("invalid selectable field path: %s, %s", fieldPath, val)
			}

			typeStr, err := getCUEValueKindString(lookup)
			if err != nil {
				return nil, fmt.Errorf("invalid selectable field '%s': %w", s, err)
			}

			fields = append(fields, templates.SchemaMetadataSelectableField{
				Field:                s,
				Optional:             optional,
				Type:                 typeStr,
				OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, fieldPath),
			})
		} else {
			return nil, fmt.Errorf("invalid selectable field path: %s, error: %w", s, valErr)
		}
	}
	return fields, nil
}

// lookupInDisjunction searches for path in each variant of a disjunction.
// Three strategies are needed to cover all cases:
//
//  1. Inline disjunctions ({A}|{B}): val.Expr() returns OrOp; iterate variants directly.
//  2. Named definitions (#A|#B), field in some variants: val.Expr() returns SelectorOp so
//     iterate doesn't work; instead unify with {field: _} to narrow to only the variants
//     that contain the field, then look up on the narrowed (single-variant) result.
//  3. Named definitions, field in ALL variants: strategy 2 still leaves a disjunction, so
//     the lookup still fails. Unify with typed constraints ({field: string} etc.) — closed
//     definitions reject non-existent fields with an error, so the constraint that succeeds
//     tells us the type. The And-constraint variant of the narrowed result carries the type.
func lookupInDisjunction(v cue.Value, path cue.Path, field string) cue.Value {
	// Strategy 1: inline disjunction.
	if op, variants := v.Expr(); op == cue.OrOp {
		for _, variant := range variants {
			if l, _ := lookupInValue(variant, path); l.Exists() {
				return l
			}
		}
	}
	// Strategy 2: named definitions, field in some variants.
	constraint := v.Context().CompileString(fmt.Sprintf("{%s: _}", field))
	if narrowed := v.Unify(constraint); narrowed.Err() == nil {
		if l, _ := lookupInValue(narrowed, path); l.Exists() {
			return l
		}
		// Strategy 3: named definitions, field in all variants. The narrowed value is still
		// a disjunction, so try scalar-typed constraints to identify the type. Closed
		// definitions produce an error for wrong types or non-existent fields, so only the
		// correct type constraint will succeed and expose the field via the And-constraint variant.
		for _, typeStr := range []string{"string", "bool", "int"} {
			typedConstraint := v.Context().CompileString(fmt.Sprintf("{%s: %s}", field, typeStr))
			if typedNarrowed := v.Unify(typedConstraint); typedNarrowed.Err() == nil {
				_, variants := typedNarrowed.Expr()
				for _, variant := range variants {
					if l, _ := lookupInValue(variant, path); l.Exists() {
						return l
					}
				}
			}
		}
	}
	return cue.Value{}
}

func lookupInValue(v cue.Value, path cue.Path) (lookup cue.Value, isOptional bool) {
	if l := v.LookupPath(path); l.Exists() {
		return l, false
	}
	if l := v.LookupPath(path.Optional()); l.Exists() {
		return l, true
	}
	return cue.Value{}, false
}

func getCUEValueKindString(v cue.Value) (string, error) {
	// Check for time.Time first via string representation (it's a struct kind in CUE)
	roughType := CUEValueToString(v)
	if strings.Contains(roughType, "time.Time") {
		return "time", nil
	}
	// Use IncompleteKind to resolve the base kind for wrapper/constrained types
	// (e.g. a string with a regex constraint formats without the word "string")
	switch v.IncompleteKind() {
	case cue.StringKind:
		return "string", nil
	case cue.BoolKind:
		return "bool", nil
	case cue.IntKind:
		return "int", nil
	}
	return "", fmt.Errorf("unsupported type %s, supported types are string, bool, int and time.Time", v.IncompleteKind())
}

// getOptionalFieldsInPath returns a list of all optional fields found along the provided fieldPath.
// This is used to generate nil checks on optional fields ensuring safe access to the selectable field.
func getOptionalFieldsInPath(v cue.Value, fieldPath string) []string {
	optionalFields := make([]string, 0)
	currentPath := make([]string, 0)

	for part := range strings.SplitSeq(fieldPath, ".") {
		currentPath = append(currentPath, part)
		cuePath := cue.MakePath(cue.Str(part))

		if lookup := v.LookupPath(cuePath); lookup.Exists() {
			v = lookup
		} else if optional := v.LookupPath(cuePath.Optional()); optional.Exists() {
			v = optional
			optionalFields = append(optionalFields, strings.Join(currentPath, "."))
		}
	}

	return optionalFields
}
