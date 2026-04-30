//nolint:dupl
package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"

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
		if err := val.Err(); err != nil {
			return nil, fmt.Errorf("invalid selectable field path: %s, error: %w", s, err)
		}

		cuePath := cue.MakePath(cue.Str(field))
		lookup, optional := lookupInValue(val, cuePath)

		// Lookup fails when the parent is a disjunction (CUE rejects field
		// access on union values); fall back to per-variant access metadata.
		if !lookup.Exists() {
			unionOptional := isFieldOptional(kind.Schema, parts)
			union, typeStr, err := buildUnionFieldAccess(val, parts, field, unionOptional)
			if err != nil {
				return nil, fmt.Errorf("invalid selectable field '%s': %w", s, err)
			}
			if union != nil {
				fields = append(fields, templates.SchemaMetadataSelectableField{
					Field:                s,
					Optional:             true,
					Type:                 typeStr,
					OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, strings.Join(parts[:len(parts)-1], ".")),
					Union:                union,
				})
				continue
			}
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
	}
	return fields, nil
}

// buildUnionFieldAccess returns metadata for accessing field through a CUE
// disjunction. Concrete-string variant fields become ConstantValue (resolved at
// codegen); typed variant fields become FieldInVariant (read at runtime).
func buildUnionFieldAccess(val cue.Value, parts []string, field string, unionOptional bool) (*templates.UnionFieldAccess, string, error) {
	variants := disjunctionVariants(val)
	if len(variants) == 0 {
		return nil, "", nil
	}

	access := &templates.UnionFieldAccess{
		UnionFieldPath: strings.Join(parts, "."),
		UnionOptional:  unionOptional,
	}

	cuePath := cue.MakePath(cue.Str(field))
	var typeStr string
	for _, variant := range variants {
		fieldVal, fieldOptional := lookupInValue(variant, cuePath)
		if !fieldVal.Exists() {
			continue
		}

		goName := variantGoName(variant)
		if goName == "" {
			return nil, "", fmt.Errorf("could not determine Go name for disjunction variant containing field %q", field)
		}

		v := templates.UnionVariantAccess{VariantField: goName}
		if fieldVal.Kind() == cue.StringKind {
			if literal, err := fieldVal.String(); err == nil {
				v.ConstantValue = literal
				if typeStr == "" {
					typeStr = "string"
				}
				access.Variants = append(access.Variants, v)
				continue
			}
		}

		ts, err := getCUEValueKindString(fieldVal)
		if err != nil {
			return nil, "", err
		}
		if typeStr == "" {
			typeStr = ts
		}
		v.FieldInVariant = upperCamelCase(field)
		v.FieldInVariantOptional = fieldOptional
		access.Variants = append(access.Variants, v)
	}

	if len(access.Variants) == 0 {
		return nil, "", nil
	}
	return access, typeStr, nil
}

// disjunctionVariants returns the OrOp branches of a CUE disjunction value.
// In the post-Decode codegen pipeline the value often arrives wrapped in an
// AndOp (intersection with a manifest open-struct constraint) whose operand
// still carries the ReferencePath to the named disjunction; unwrap AndOp and
// follow references before falling back to evaluation.
func disjunctionVariants(v cue.Value) []cue.Value {
	if variants := variantsFromValue(v); len(variants) > 0 {
		return variants
	}
	if eval := v.Eval(); eval.Err() == nil && !sameValue(eval, v) {
		if variants := variantsFromValue(eval); len(variants) > 0 {
			return variants
		}
	}
	return nil
}

func variantsFromValue(v cue.Value) []cue.Value {
	op, args := v.Expr()
	switch op {
	case cue.OrOp:
		return args
	case cue.AndOp:
		for _, arg := range args {
			if variants := variantsFromValue(arg); len(variants) > 0 {
				return variants
			}
		}
	default:
		// fall through to ReferencePath resolution
	}
	if root, ref := v.ReferencePath(); len(ref.Selectors()) > 0 {
		referenced := root.LookupPath(ref)
		if referenced.Exists() && !sameValue(referenced, v) {
			return variantsFromValue(referenced)
		}
	}
	return nil
}

// sameValue guards against ReferencePath resolving back to v itself.
func sameValue(a, b cue.Value) bool {
	return a.Path().String() == b.Path().String()
}

// variantGoName returns the cog-generated Go field name for a disjunction
// variant: the definition name for #Foo, or
// UpperCamelCase(discField)+UpperCamelCase(discValue) for anonymous variants
// (matches cog's DisjunctionOfAnonymousStructsToExplicit pass).
func variantGoName(variant cue.Value) string {
	if name := definitionName(variant); name != "" {
		return name
	}
	iter, err := variant.Fields(cue.Optional(true))
	if err != nil {
		return ""
	}
	for iter.Next() {
		fv := iter.Value()
		if !fv.IsConcrete() || fv.Kind() != cue.StringKind {
			continue
		}
		if s, err := fv.String(); err == nil {
			return upperCamelCase(iter.Selector().String()) + upperCamelCase(s)
		}
	}
	return ""
}

// definitionName extracts a CUE definition name (#Foo → "Foo") from a variant.
// Variants of a named disjunction (#Union: #A | #B) report Path()=#Union but
// ReferencePath()=#A; check ReferencePath first, fall back to Source AST.
func definitionName(variant cue.Value) string {
	if _, ref := variant.ReferencePath(); len(ref.Selectors()) > 0 {
		sels := ref.Selectors()
		if name := defNameFromSelector(sels[len(sels)-1]); name != "" {
			return name
		}
	}
	if id, ok := variant.Source().(*ast.Ident); ok {
		if strings.HasPrefix(id.Name, "#") {
			return id.Name[1:]
		}
	}
	return ""
}

func defNameFromSelector(sel cue.Selector) string {
	s := sel.String()
	if strings.HasPrefix(s, "#") {
		return s[1:]
	}
	return ""
}

func upperCamelCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// isFieldOptional reports whether parts' final selector was declared optional
// (CUE `?:`). Works on fully-evaluated schemas where source AST is unavailable
// by comparing the parent's lookup with and without the optional marker.
func isFieldOptional(schema cue.Value, parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	parentSels := make([]cue.Selector, 0, len(parts)-1)
	for _, p := range parts[:len(parts)-1] {
		parentSels = append(parentSels, cue.Str(p))
	}
	parent := schema.LookupPath(cue.MakePath(parentSels...).Optional())
	if !parent.Exists() {
		return false
	}
	leaf := cue.MakePath(cue.Str(parts[len(parts)-1]))
	if parent.LookupPath(leaf).Exists() {
		return false
	}
	return parent.LookupPath(leaf.Optional()).Exists()
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
