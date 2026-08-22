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
		tc, err := s.getTableColumns(&kind)
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
			TableColumns:     tc,
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
			fullPathParts := append(append([]string{}, parts...), field)
			unionVal, unionPath, variantPath, ok := resolveUnionPath(kind.Schema, fullPathParts)
			if !ok {
				return nil, fmt.Errorf("invalid selectable field path: %s, error: %w", s, err)
			}
			unionOptional := isFieldOptional(kind.Schema, unionPath)
			union, typeStr, err := buildUnionFieldAccess(unionVal, unionPath, variantPath, unionOptional)
			if err != nil {
				return nil, fmt.Errorf("invalid selectable field '%s': %w", s, err)
			}
			if union != nil {
				if union.CollapsedNullToSingle {
					fields = append(fields, templates.SchemaMetadataSelectableField{
						Field:                s,
						Optional:             union.Variants[0].FieldInVariantOptional,
						Type:                 typeStr,
						OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, fieldPath),
					})
					continue
				}

				fields = append(fields, templates.SchemaMetadataSelectableField{
					Field:                s,
					Optional:             true,
					Type:                 typeStr,
					OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, parentPathBeforeUnion(unionPath)),
					Union:                union,
				})
				continue
			}
			return nil, fmt.Errorf("invalid selectable field path: %s, %s", fieldPath, unionVal)
		}

		cuePath := cue.MakePath(cue.Str(field))
		lookup, optional := lookupInValue(val, cuePath)

		// Lookup fails when the parent is a disjunction (CUE rejects field
		// access on union values); fall back to per-variant access metadata.
		if !lookup.Exists() {
			unionOptional := isFieldOptional(kind.Schema, parts)
			union, typeStr, err := buildUnionFieldAccess(val, parts, []string{field}, unionOptional)
			if err != nil {
				return nil, fmt.Errorf("invalid selectable field '%s': %w", s, err)
			}
			if union != nil {
				// Unions with one variant mean there was a union with a single element and the `null` literal,
				// this doesn't create a go type with union members as fields, so report this as a regular nullable field
				if union.CollapsedNullToSingle {
					fields = append(fields, templates.SchemaMetadataSelectableField{
						Field:                s,
						Optional:             union.Variants[0].FieldInVariantOptional,
						Type:                 typeStr,
						OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, fieldPath),
					})
					continue
				}

				fields = append(fields, templates.SchemaMetadataSelectableField{
					Field:                s,
					Optional:             true,
					Type:                 typeStr,
					OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, parentPathBeforeUnion(parts)),
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
//
//nolint:revive
func buildUnionFieldAccess(val cue.Value, unionPath []string, variantFieldPath []string, unionOptional bool) (*templates.UnionFieldAccess, string, error) {
	variants, hadNull := disjunctionVariants(val)
	if hadNull && !unionOptional {
		return nil, "", fmt.Errorf("disjunction at %q includes null but the field is required; declare it optional (use ?:) to allow null", strings.Join(unionPath, "."))
	}
	if len(variants) == 0 {
		return nil, "", nil
	}

	access := &templates.UnionFieldAccess{
		UnionFieldPath:        strings.Join(unionPath, "."),
		UnionOptional:         unionOptional,
		CollapsedNullToSingle: hadNull && len(variants) == 1,
	}

	cuePath := cuePathFromParts(variantFieldPath)
	var typeStr string
	for _, variant := range variants {
		fieldVal, fieldOptional := lookupInValue(variant, cuePath)
		if !fieldVal.Exists() {
			continue
		}

		goName := variantGoName(variant)
		if goName == "" {
			return nil, "", fmt.Errorf("could not determine Go name for disjunction variant containing field %q", strings.Join(variantFieldPath, "."))
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
		v.FieldInVariant = toGoFieldPath(variantFieldPath)
		v.FieldInVariantOptional = fieldOptional
		access.Variants = append(access.Variants, v)
	}

	if len(access.Variants) == 0 {
		return nil, "", nil
	}
	return access, typeStr, nil
}

// resolveUnionPath finds the first path segment that crosses a disjunction.
// It returns the disjunction value, the path to that disjunction from root,
// and the remaining path segments to resolve within each variant.
func resolveUnionPath(schema cue.Value, fullPathParts []string) (unionVal cue.Value, unionPath []string, variantPath []string, ok bool) {
	current := schema
	traversed := make([]string, 0, len(fullPathParts))
	for i, part := range fullPathParts {
		next, _ := lookupInValue(current, cue.MakePath(cue.Str(part)))
		if next.Exists() {
			current = next
			traversed = append(traversed, part)
			continue
		}

		if variants, _ := disjunctionVariants(current); len(variants) > 0 {
			return current, append([]string{}, traversed...), append([]string{}, fullPathParts[i:]...), true
		}
		return cue.Value{}, nil, nil, false
	}

	return cue.Value{}, nil, nil, false
}

func cuePathFromParts(parts []string) cue.Path {
	selectors := make([]cue.Selector, 0, len(parts))
	for _, part := range parts {
		selectors = append(selectors, cue.Str(part))
	}
	return cue.MakePath(selectors...)
}

func toGoFieldPath(parts []string) string {
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		paths = append(paths, upperCamelCase(part))
	}
	return strings.Join(paths, ".")
}

func parentPathBeforeUnion(unionPath []string) string {
	if len(unionPath) < 2 {
		return ""
	}
	return strings.Join(unionPath[:len(unionPath)-1], ".")
}

// disjunctionVariants returns the OrOp branches of a CUE disjunction value,
// with any `null` literal variants filtered out; hadNull reports whether a
// null variant was present. In the post-Decode codegen pipeline the value
// often arrives wrapped in an AndOp (intersection with a manifest open-struct
// constraint) whose operand still carries the ReferencePath to the named
// disjunction; unwrap AndOp and follow references before falling back to
// evaluation.
func disjunctionVariants(v cue.Value) (variants []cue.Value, hadNull bool) {
	if raw := variantsFromValue(v); len(raw) > 0 {
		return filterNullVariants(raw)
	}
	if eval := v.Eval(); eval.Err() == nil && !sameValue(eval, v) {
		if raw := variantsFromValue(eval); len(raw) > 0 {
			return filterNullVariants(raw)
		}
	}
	return nil, false
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

// filterNullVariants drops `null` literal variants from a disjunction's
// branches. A `T | null` disjunction conveys "field is optional"; cog
// lowers it to a nullable Go pointer and the variant itself has no Go
// counterpart, so it cannot participate in a typed variant access.
func filterNullVariants(variants []cue.Value) ([]cue.Value, bool) {
	hadNull := false
	out := variants[:0:0]
	for _, variant := range variants {
		if variant.IncompleteKind() == cue.NullKind {
			hadNull = true
			continue
		}
		out = append(out, variant)
	}
	return out, hadNull
}

// sameValue guards against ReferencePath resolving back to v itself.
func sameValue(a, b cue.Value) bool {
	return a.Path().String() == b.Path().String()
}

// variantGoName returns the cog-generated Go field name for a disjunction
// variant: the definition name for #Foo, or
// UpperCamelCase(discValue)+UpperCamelCase(discField) for anonymous variants
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
			return upperCamelCase(s) + upperCamelCase(iter.Selector().String())
		}
	}
	// Return the type name
	if _, ref := variant.ReferencePath(); len(ref.Selectors()) > 0 {
		sels := ref.Selectors()
		return sels[len(sels)-1].String()[1:]
	}
	return ""
}

// definitionName extracts a CUE definition name (#Foo → "Foo") from a variant.
// Variants of a named disjunction (#Union: #A | #B) report Path()=#Union but
// ReferencePath()=#A; check ReferencePath first, fall back to Source AST.
func definitionName(variant cue.Value) string {
	if _, ref := variant.ReferencePath(); len(ref.Selectors()) > 0 {
		sels := ref.Selectors()
		if s := sels[len(sels)-1].String(); strings.HasPrefix(s, "#") {
			return s[1:]
		}
	}
	if id, ok := variant.Source().(*ast.Ident); ok {
		if strings.HasPrefix(id.Name, "#") {
			return id.Name[1:]
		}
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

func (*SchemaGenerator) getTableColumns(kind *codegen.VersionedKind) ([]templates.SchemaMetadataTableColumn, error) {
	columns := make([]templates.SchemaMetadataTableColumn, 0)
	if len(kind.AdditionalPrinterColumns) == 0 {
		return columns, nil
	}
	for _, col := range kind.AdditionalPrinterColumns {
		fieldPath := col.JSONPath
		if len(fieldPath) > 1 && fieldPath[0] == '.' {
			fieldPath = fieldPath[1:]
		}
		parts := strings.Split(fieldPath, ".")
		if len(parts) <= 1 {
			return nil, fmt.Errorf("invalid table column JSONPath: %s", col.JSONPath)
		}
		field := parts[len(parts)-1]
		parentParts := parts[:len(parts)-1]
		path := make([]cue.Selector, 0)
		for _, p := range parentParts {
			path = append(path, cue.Str(p))
		}
		val := kind.Schema.LookupPath(cue.MakePath(path...).Optional())
		if val.Err() != nil {
			return nil, fmt.Errorf("invalid table column JSONPath %s: parent path not found", col.JSONPath)
		}

		var lookup cue.Value
		var optional bool
		cuePath := cue.MakePath(cue.Str(field))
		if lookup = val.LookupPath(cuePath); lookup.Exists() {
			optional = false
		} else if lookup = val.LookupPath(cuePath.Optional()); lookup.Exists() {
			optional = true
		} else {
			return nil, fmt.Errorf("invalid table column JSONPath: %s", col.JSONPath)
		}

		goType, err := getCUEValueKindString(lookup)
		if err != nil {
			return nil, fmt.Errorf("invalid table column '%s' (%s): %w", col.Name, col.JSONPath, err)
		}

		var colFormat string
		if col.Format != nil {
			colFormat = *col.Format
		}
		var colDescription string
		if col.Description != nil {
			colDescription = *col.Description
		}
		var priority int32
		if col.Priority != nil {
			priority = *col.Priority
		}

		columns = append(columns, templates.SchemaMetadataTableColumn{
			Name:                 col.Name,
			Type:                 col.Type,
			Format:               colFormat,
			Description:          colDescription,
			Priority:             priority,
			JSONPath:             col.JSONPath,
			GoValueType:          goType,
			Optional:             optional,
			OptionalFieldsInPath: getOptionalFieldsInPath(kind.Schema, fieldPath),
		})
	}
	return columns, nil
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
