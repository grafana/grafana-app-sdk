//
// Code generated by grafana-app-sdk. DO NOT EDIT.
//

package v1

import (
	"github.com/grafana/grafana-app-sdk/resource"
)

// schema is unexported to prevent accidental overwrites
var (
	schemaExternalName = resource.NewSimpleSchema("core.grafana.internal", "v1", &ExternalName{}, &ExternalNameList{}, resource.WithKind("ExternalName"),
		resource.WithPlural("externalnames"), resource.WithScope(resource.ClusterScope))
	kindExternalName = resource.Kind{
		Schema: schemaExternalName,
		Codecs: map[resource.KindEncoding]resource.Codec{
			resource.KindEncodingJSON: &ExternalNameJSONCodec{},
		},
	}
)

// Kind returns a resource.Kind for this Schema with a JSON codec
func ExternalNameKind() resource.Kind {
	return kindExternalName
}

// Schema returns a resource.SimpleSchema representation of ExternalName
func ExternalNameSchema() *resource.SimpleSchema {
	return schemaExternalName
}

// Interface compliance checks
var _ resource.Schema = kindExternalName