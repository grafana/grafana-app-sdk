//
// Code generated by grafana-app-sdk. DO NOT EDIT.
//

package v1

import (
	"github.com/grafana/grafana-app-sdk/resource"
)

// schema is unexported to prevent accidental overwrites
var (
	schemaTestKind = resource.NewSimpleSchema("core.grafana.internal", "v1", &TestKind{}, &TestKindList{}, resource.WithKind("TestKind"),
		resource.WithPlural("testkinds"), resource.WithScope(resource.NamespacedScope))
	kindTestKind = resource.Kind{
		Schema: schemaTestKind,
		Codecs: map[resource.KindEncoding]resource.Codec{
			resource.KindEncodingJSON: &TestKindJSONCodec{},
		},
	}
)

// Kind returns a resource.Kind for this Schema with a JSON codec
func TestKindKind() resource.Kind {
	return kindTestKind
}

// Schema returns a resource.SimpleSchema representation of TestKind
func TestKindSchema() *resource.SimpleSchema {
	return schemaTestKind
}

// Interface compliance checks
var _ resource.Schema = kindTestKind
