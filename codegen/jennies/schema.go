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
	files := make(codejen.Files, 0)
	for _, version := range appManifest.Versions() {
		for _, kind := range version.Kinds() {
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
		if val := kind.Schema.LookupPath(cue.MakePath(path...).Optional()); val.Err() == nil {
			var lookup cue.Value
			var optional bool

			cuePath := cue.MakePath(cue.Str(field))

			// Simplest way to check if it's an optional field is to try to look it up as non-optional, then try optional
			if lookup = val.LookupPath(cuePath); lookup.Exists() {
				optional = false
			} else if lookup = val.LookupPath(cuePath.Optional()); lookup.Exists() {
				optional = true
			} else {
				return nil, fmt.Errorf("invalid selectable field path: %s", fieldPath)
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
	}
	return fields, nil
}

func getCUEValueKindString(v cue.Value) (string, error) {
	// This is a kind of messy way of guessing type without having to actually parse the AST
	roughType := CUEValueToString(v)
	switch {
	case strings.Contains(roughType, "time.Time"):
		return "time", nil
	case strings.Contains(roughType, "string"):
		return "string", nil
	case strings.Contains(roughType, "bool"):
		return "bool", nil
	case strings.Contains(roughType, "int"):
		return "int", nil
	}
	return "", fmt.Errorf("unsupported type %s, supported types are string, bool, int and time.Time", v.Kind())
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
