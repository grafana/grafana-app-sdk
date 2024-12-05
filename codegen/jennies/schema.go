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
// nolint:dupl
func (s *SchemaGenerator) Generate(kind codegen.Kind) (codejen.Files, error) {
	meta := kind.Properties()

	if meta.Scope != string(resource.NamespacedScope) && meta.Scope != string(resource.ClusterScope) {
		return nil, fmt.Errorf("scope '%s' is invalid, must be one of: '%s', '%s'",
			meta.Scope, resource.ClusterScope, resource.NamespacedScope)
	}

	prefix := ""
	if !s.GroupByKind {
		prefix = exportField(kind.Name())
	}

	files := make(codejen.Files, 0)
	for _, ver := range kind.Versions() {
		sf, err := s.getSelectableFields(&ver)
		if err != nil {
			return nil, err
		}
		b := bytes.Buffer{}
		err = templates.WriteSchema(templates.SchemaMetadata{
			Package:          ToPackageName(ver.Version),
			Group:            meta.Group,
			Version:          ver.Version,
			Kind:             meta.Kind,
			Plural:           meta.PluralMachineName,
			Scope:            meta.Scope,
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
			RelativePath: filepath.Join(GetGeneratedPath(s.GroupByKind, kind, ver.Version), fmt.Sprintf("%s_schema_gen.go", meta.MachineName)),
			From:         []codejen.NamedJenny{s},
		})
	}

	return files, nil
}

func (*SchemaGenerator) getSelectableFields(ver *codegen.KindVersion) ([]templates.SchemaMetadataSeletableField, error) {
	fields := make([]templates.SchemaMetadataSeletableField, 0)
	if len(ver.SelectableFields) == 0 {
		return fields, nil
	}
	// Check each field in the CUE (TODO: make this OpenAPI instead?) to check if the field is optional
	for _, s := range ver.SelectableFields {
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
		if val := ver.Schema.LookupPath(cue.MakePath(path...).Optional()); val.Err() == nil {
			// Simplest way to check if it's an optional field is to try to look it up as non-optional, then try optional
			if lookup := val.LookupPath(cue.MakePath(cue.Str(field))); lookup.Exists() {
				fields = append(fields, templates.SchemaMetadataSeletableField{
					Field:    s,
					Optional: false,
				})
			} else if optional := val.LookupPath(cue.MakePath(cue.Str(field).Optional())); optional.Exists() {
				fields = append(fields, templates.SchemaMetadataSeletableField{
					Field:    s,
					Optional: true,
				})
			} else {
				return nil, fmt.Errorf("invalid selectable field path: %s", fieldPath)
			}
		}
	}
	return fields, nil
}
