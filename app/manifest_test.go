package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func TestManifestData_Validate(t *testing.T) {
	tests := []struct {
		name        string
		data        ManifestData
		expectedErr error
	}{{
		name: "valid (empty manifest)",
		data: ManifestData{},
	}, {
		name: "plural mismatch",
		data: ManifestData{
			Versions: []ManifestVersion{{
				Name: "v1",
				Kinds: []ManifestVersionKind{{
					Kind:   "Foo",
					Plural: "foos",
				}},
			}, {
				Name: "v2",
				Kinds: []ManifestVersionKind{{
					Kind:   "Foo",
					Plural: "bars",
				}},
			}},
		},
		expectedErr: multierror.Append(nil, errors.New("kind 'Foo' has a different plural in versions 'v1' and 'v2'")),
	}, {
		name: "scope mismatch",
		data: ManifestData{
			Versions: []ManifestVersion{{
				Name: "v1",
				Kinds: []ManifestVersionKind{{
					Kind:  "Foo",
					Scope: "Namespaced",
				}},
			}, {
				Name: "v2",
				Kinds: []ManifestVersionKind{{
					Kind:  "Foo",
					Scope: "Cluster",
				}},
			}},
		},
		expectedErr: multierror.Append(nil, errors.New("kind 'Foo' has a different scope in versions 'v1' and 'v2'")),
	}, {
		name: "conversion mismatch",
		data: ManifestData{
			Versions: []ManifestVersion{{
				Name: "v1",
				Kinds: []ManifestVersionKind{{
					Kind:       "Foo",
					Conversion: true,
				}},
			}, {
				Name: "v2",
				Kinds: []ManifestVersionKind{{
					Kind:       "Foo",
					Conversion: false,
				}},
			}},
		},
		expectedErr: multierror.Append(nil, errors.New("kind 'Foo' conversion does not match in versions 'v1' and 'v2'")),
	}, {
		name: "plural, scope, and conversion mismatch",
		data: ManifestData{
			Versions: []ManifestVersion{{
				Name: "v1",
				Kinds: []ManifestVersionKind{{
					Kind:       "Foo",
					Plural:     "foos",
					Scope:      "Namespaced",
					Conversion: true,
				}},
			}, {
				Name: "v2",
				Kinds: []ManifestVersionKind{{
					Kind:       "Foo",
					Plural:     "bars",
					Scope:      "Cluster",
					Conversion: false,
				}},
			}},
		},
		expectedErr: multierror.Append(nil,
			errors.New("kind 'Foo' has a different plural in versions 'v1' and 'v2'"),
			errors.New("kind 'Foo' has a different scope in versions 'v1' and 'v2'"),
			errors.New("kind 'Foo' conversion does not match in versions 'v1' and 'v2'")),
	}, {
		name: "conflicting routes and kinds",
		data: ManifestData{
			Versions: []ManifestVersion{{
				Name: "v1",
				Kinds: []ManifestVersionKind{{
					Kind:   "Foo",
					Plural: "foos",
					Scope:  "Namespaced",
				}, {
					Kind:   "Bar",
					Plural: "bars",
					Scope:  "Cluster",
				}, {
					Kind:   "Foobar",
					Plural: "foobars",
					Scope:  "Cluster",
				}},
				Routes: ManifestVersionRoutes{
					Namespaced: map[string]spec3.PathProps{
						"/foos": spec3.PathProps{},
						"/bars": spec3.PathProps{},
					},
					Cluster: map[string]spec3.PathProps{
						"/foobars": spec3.PathProps{},
					},
				},
			}},
		},
		expectedErr: multierror.Append(nil,
			errors.New("namespaced custom route '/foos' conflicts with already-registered kind 'foos'"),
			errors.New("cluster-scoped custom route '/foobars' conflicts with already-registered kind 'foobars'")),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedErr, test.data.Validate())
		})
	}
}

func TestVersionSchemaFromMap(t *testing.T) {
	tests := []struct {
		name        string
		schema      map[string]any
		kind        string
		expectedMap map[string]any
		expectedErr error
	}{{
		name:        "valid (empty map)",
		schema:      map[string]any{},
		kind:        "Foo",
		expectedMap: map[string]any{},
		expectedErr: nil,
	}, {
		name:        "CRD shape",
		schema:      jsonToMap([]byte(`{"spec":{"properties":{"foo":"string"},"type":"object"}}`)),
		kind:        "Bar",
		expectedMap: jsonToMap([]byte(`{"Bar":{"type":"object","properties":{"spec":{"properties":{"foo":"string"},"type":"object"}}}}`)),
	}, {
		name:        "Full CRD",
		schema:      jsonToMap([]byte(`{"openAPIV3Schema":{"properties":{"spec":{"properties":{"foo":"string"},"type":"object"}}}}`)),
		kind:        "Foo",
		expectedMap: jsonToMap([]byte(`{"Foo":{"type":"object","properties":{"spec":{"properties":{"foo":"string"},"type":"object"}}}}`)),
	}, {
		name:        "OpenAPI without kind",
		schema:      jsonToMap([]byte(`{"components":{"schemas":{"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"bar":{"type":"string"}}}}}}}}`)),
		kind:        "Bar",
		expectedErr: errors.New("kind \"Bar\" not found in map openAPI components"),
	}, {
		name:        "OpenAPI without references",
		schema:      jsonToMap([]byte(`{"components":{"schemas":{"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"bar":{"type":"string"}}}}}}}}`)),
		kind:        "Foo",
		expectedMap: jsonToMap([]byte(`{"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"bar":{"type":"string"}}}}}}`)),
	}, {
		name:        "OpenAPI with references",
		schema:      jsonToMap([]byte(`{"components":{"schemas":{"Bar":{"type":"object","properties":{"foo":{"type":"string"}}},"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"bar":{"type":"string"},"ref":{"$ref":"#/components/schemas/Bar"}}}}}}}}`)),
		kind:        "Foo",
		expectedMap: jsonToMap([]byte(`{"Bar":{"type":"object","properties":{"foo":{"type":"string"}}},"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"bar":{"type":"string"},"ref":{"$ref":"#/components/schemas/Bar"}}}}}}`)),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sch, err := VersionSchemaFromMap(test.schema, test.kind)
			if test.expectedErr == nil {
				require.NoError(t, err)
				assert.Equal(t, test.expectedMap, sch.AsOpenAPI3SchemasMap())
			} else {
				assert.Equal(t, test.expectedErr, err)
			}
		})
	}
}

func jsonToMap(data []byte) map[string]any {
	m := make(map[string]any)
	_ = json.Unmarshal(data, &m)
	return m
}

func TestVersionSchema_AsKubeOpenAPI(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "test.grafana.app",
		Version: "v1",
		Kind:    "Foo",
	}
	ref := func(path string) spec.Ref {
		r, e := spec.NewRef(path)
		if e != nil {
			panic(e)
		}
		return r
	}
	tests := []struct {
		name   string
		schema []byte
		gvk    schema.GroupVersionKind
		ref    common.ReferenceCallback
		want   map[string]common.OpenAPIDefinition
		err    error
	}{{
		name:   "empty",
		schema: []byte(`{}`),
		gvk:    gvk,
		ref:    ref,
		err:    errors.New("unable to locate openAPI definition for kind Foo"),
	}, {
		name:   "spec (CRD-shape)",
		schema: []byte(`{"spec":{"type":"object","properties":{"foo":{"type":"string"}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
		err: nil,
	}, {
		name:   "spec (OpenAPI-shape)",
		schema: []byte(`{"components":{"schemas":{"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"foo":{"type":"string"}}}}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
		err: nil,
	}, {
		name:   "dependencies (CRD-shape)",
		schema: []byte(`{"#foo":{"type":"object","properties":{"foobar":{"type":"string"}}},"spec":{"type":"object","properties":{"foo":{"type":"string"},"bar":{"$ref":"#/components/schemas/#foo"}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
						"bar": {
							SchemaProps: spec.SchemaProps{
								Ref: ref("test.grafana.app/v1.Foo#foo"),
							},
						},
					},
				},
			}, "test.grafana.app/v1.Foo#foo"),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.Foo#foo": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						Properties: map[string]spec.Schema{
							"foobar": {
								SchemaProps: spec.SchemaProps{
									Type: []string{"string"},
								},
							},
						},
					},
				},
			},
		},
	}, {
		name:   "dependencies (OpenAPI-shape)",
		schema: []byte(`{"components":{"schemas":{"#foo":{"type":"object","properties":{"foobar":{"type":"string"}}},"Foo":{"type":"object","properties":{"spec":{"type":"object","properties":{"foo":{"type":"string"},"bar":{"$ref":"#/components/schemas/#foo"}}}}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
						"bar": {
							SchemaProps: spec.SchemaProps{
								Ref: ref("test.grafana.app/v1.Foo#foo"),
							},
						},
					},
				},
			}, "test.grafana.app/v1.Foo#foo"),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.Foo#foo": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						Properties: map[string]spec.Schema{
							"foobar": {
								SchemaProps: spec.SchemaProps{
									Type: []string{"string"},
								},
							},
						},
					},
				},
			},
		},
	}, {
		name:   "additional subresources (CRD-shape)",
		schema: []byte(`{"status":{"type":"object","properties":{"foobar":{"type":"string"}}},"spec":{"type":"object","properties":{"foo":{"type":"string"}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
				"status": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foobar": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name:   "additional subresources (OpenAPI-shape)",
		schema: []byte(`{"components":{"schemas":{"Foo":{"type":"object","properties":{"status":{"type":"object","properties":{"foobar":{"type":"string"}}},"spec":{"type":"object","properties":{"foo":{"type":"string"}}}}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
				"status": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"foobar": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name:   "freeform additionalProperties (CRD-shape)",
		schema: []byte(`{"spec":{"type":"object","additionalProperties":true}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name:   "freeform additionalProperties (OpenAPI-shape)",
		schema: []byte(`{"components":{"schemas":{"Foo":{"type":"object","properties":{"spec":{"type":"object","additionalProperties":true}}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name:   "defined additionalProperties (CRD-shape)",
		schema: []byte(`{"spec":{"type":"object","additionalProperties":{"type":"object","properties":{"foo":{"type":"string"}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Schema: &spec.Schema{
							SchemaProps: spec.SchemaProps{
								Type: spec.StringOrArray{"object"},
								Properties: map[string]spec.Schema{
									"foo": {
										SchemaProps: spec.SchemaProps{
											Type: []string{"string"},
										},
									},
								},
							},
						},
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name:   "defined additionalProperties (OpenAPI-shape)",
		schema: []byte(`{"spec":{"type":"object","additionalProperties":{"type":"object","properties":{"foo":{"type":"string"}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Schema: &spec.Schema{
							SchemaProps: spec.SchemaProps{
								Type: spec.StringOrArray{"object"},
								Properties: map[string]spec.Schema{
									"foo": {
										SchemaProps: spec.SchemaProps{
											Type: []string{"string"},
										},
									},
								},
							},
						},
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name:   "defined additionalProperties",
		schema: []byte(`{"components":{"schemas":{"Foo":{"properties":{"spec":{"type":"object","x-kubernetes-preserve-unknown-fields":true}}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
					},
				},
			}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
	}, {
		name: "full complex schema",
		schema: []byte(`{
	"components":{
		"schemas":{
			"#foo":{
				"type":"object",
				"x-kubernetes-preserve-unknown-fields":true
			},
			"#bar":{
				"type":"object",
				"properties": {
					"int": {
						"type":"number",
						"minimum": 5,
						"maximum": 10,
						"format": "integer"
					},
					"string": { "type":"string" },
					"bool": { "type":"boolean", "default": true },
					"float": { 
						"type":"number",
						"minimum": -0.5,
						"maximum": 0.5,
						"format": "decimal"
					}
				}
			},
			"#foobar":{
				"type":"string"
			},
			"Foo": {
				"type": "object",
				"properties": {
					"spec":{
						"type":"object",
						"properties":{
							"all": {
								"allOf": [{"$ref":"#/components/schemas/#foo"},{"$ref":"#/components/schemas/#bar"}]
							},
							"any": {
								"anyOf": [{"$ref":"#/components/schemas/#foo"},{"$ref":"#/components/schemas/#bar"},{"$ref":"#/components/schemas/#foobar"}]
							},
							"one": {
								"oneOf": [{"$ref":"#/components/schemas/#foo"},{"$ref":"#/components/schemas/#foobar"}]
							},
							"no": {
								"not": {
									"$ref": "#/components/schemas/#foo"
								}
							},
							"array": {
								"type":  "array",
								"items": {
									"type": "string"
								}
							},
							"refarray": {
								"type": "array",
								"items": {
									"$ref": "#/components/schemas/#foo"
								}
							}
						}
					},
					"status":{"type":"object","x-kubernetes-preserve-unknown-fields":true}
				}
			}
		}
	}
}`),
		gvk: gvk,
		ref: ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo": kubeOpenAPIKindWithProps(gvk, ref, map[string]spec.SchemaProps{
				"spec": {
					Type: spec.StringOrArray{"object"},
					Properties: map[string]spec.Schema{
						"all": {
							SchemaProps: spec.SchemaProps{
								AllOf: []spec.Schema{{
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#foo"),
									},
								}, {
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#bar"),
									},
								}},
							},
						},
						"any": {
							SchemaProps: spec.SchemaProps{
								AnyOf: []spec.Schema{{
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#foo"),
									},
								}, {
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#bar"),
									},
								}, {
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#foobar"),
									},
								}},
							},
						},
						"one": {
							SchemaProps: spec.SchemaProps{
								OneOf: []spec.Schema{{
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#foo"),
									},
								}, {
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#foobar"),
									},
								}},
							},
						},
						"no": {
							SchemaProps: spec.SchemaProps{
								Not: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: ref("test.grafana.app/v1.Foo#foo"),
									},
								},
							},
						},
						"array": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"array"},
								Items: &spec.SchemaOrArray{
									Schema: &spec.Schema{
										SchemaProps: spec.SchemaProps{
											Type: spec.StringOrArray{"string"},
										},
									},
								},
							},
						},
						"refarray": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"array"},
								Items: &spec.SchemaOrArray{
									Schema: &spec.Schema{
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.Foo#foo"),
										},
									},
								},
							},
						},
					},
				},
				"status": {
					Type: spec.StringOrArray{"object"},
					AdditionalProperties: &spec.SchemaOrBool{
						Allows: true,
					},
				},
			}, "test.grafana.app/v1.Foo#bar", "test.grafana.app/v1.Foo#foo", "test.grafana.app/v1.Foo#foobar"),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.Foo#foo": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						AdditionalProperties: &spec.SchemaOrBool{
							Allows: true,
						},
					},
				},
			},
			"test.grafana.app/v1.Foo#bar": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						Properties: map[string]spec.Schema{
							"int": {
								SchemaProps: spec.SchemaProps{
									Type:    []string{"number"},
									Minimum: ptr(float64(5)),
									Maximum: ptr(float64(10)),
									Format:  "integer",
								},
							},
							"string": {
								SchemaProps: spec.SchemaProps{
									Type: []string{"string"},
								},
							},
							"bool": {
								SchemaProps: spec.SchemaProps{
									Type:    []string{"boolean"},
									Default: true,
								},
							},
							"float": {
								SchemaProps: spec.SchemaProps{
									Type:    []string{"number"},
									Minimum: ptr(float64(-0.5)),
									Maximum: ptr(float64(0.5)),
									Format:  "decimal",
								},
							},
						},
					},
				},
			},
			"test.grafana.app/v1.Foo#foobar": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"string"},
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := make(map[string]any)
			require.Nil(t, json.Unmarshal(test.schema, &m))
			vs, err := VersionSchemaFromMap(m, test.gvk.Kind)
			require.Nil(t, err)
			res, err := vs.AsKubeOpenAPI(test.gvk, test.ref, "test.grafana.app/v1")
			if test.err != nil {
				assert.Equal(t, test.err, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.want, res)
			}
		})
	}
}

func TestGetCRDOpenAPISchema(t *testing.T) {
	tests := []struct {
		name          string
		schemaName    string
		jsonData      []byte
		outputJSON    []byte
		expectedError error
	}{{
		name:          "empty document",
		schemaName:    "foo",
		jsonData:      []byte(`{}`),
		expectedError: fmt.Errorf("invalid components or schemas"),
	}, {
		name:          "missing schema",
		schemaName:    "foo",
		jsonData:      []byte(`{"components":{"schemas":{}}}`),
		expectedError: fmt.Errorf("schema foo not found"),
	}, {
		name:       "recursive schema",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"item":{"type":"object","properties":{"val":{"type":"string"},"next":{"type":"object","$ref":"#/components/schemas/item"}}},"foo":{"type":"object","properties":{"linked":{"type":"object","$ref":"#/components/schemas/item"}}}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"linked":{"type":"object","properties":{"val":{"type":"string"},"next":{"type":"object","x-kubernetes-preserve-unknown-fields":true}}}}}`),
	}, {
		name:       "additionalProperties replaced",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"type":"object","additionalProperties":{"type":"object"}}}}}`),
		outputJSON: []byte(`{"type":"object","x-kubernetes-preserve-unknown-fields":true}`),
	}, {
		name:       "additionalProperties resolved",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"bar":{"type":"object","properties":{"baz":{"type":"string"}}},"foo":{"type":"object","additionalProperties":{"type":"object","$ref":"#/components/schemas/bar"}}}}}`),
		outputJSON: []byte(`{"type":"object","additionalProperties":{"type":"object","properties":{"baz":{"type":"string"}}}}`),
	}, {
		name:       "simple additionalProperties",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"type":"object","properties":{"bar":{"type":"string"}},"additionalProperties":{}}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"bar":{"type":"string"}},"x-kubernetes-preserve-unknown-fields":true}`),
	}, {
		name:       "convert to structural schema: oneOf",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"oneOf":[{"type":"object","properties":{"foo":{"type":"string"}},"required":["foo"]},{}]}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"foo":{"type":"string"}},"oneOf":[{"required":["foo"]},{"not":{"anyOf":[{"required":["foo"]}]}}]}`),
	}, {
		name:       "convert to structural schema: anyOf",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"anyOf":[{"type":"object","properties":{"foo":{"type":"string"}},"required":["foo"]},{"properties":{"bar":{"type":"string"}},"required":["bar"]}]}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"foo":{"type":"string"},"bar":{"type":"string"}},"anyOf":[{"required":["foo"]},{"required":["bar"]}]}`),
	}, {
		name:       "convert to structural schema: allOf",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"allOf":[{"type":"object","properties":{"foo":{"type":"string"}},"required":["foo"]},{"properties":{"bar":{"type":"string"}},"required":["bar"]}]}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"foo":{"type":"string"},"bar":{"type":"string"}},"allOf":[{"required":["foo"]},{"required":["bar"]}]}`),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := openapi3.NewLoader().LoadFromData(test.jsonData)
			assert.NoError(t, err)
			output, err := GetCRDOpenAPISchema(doc.Components, test.schemaName)
			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, err)
			} else {
				require.NoError(t, err)
				m, err := json.MarshalIndent(output, "", "  ")
				require.NoError(t, err)
				assert.JSONEq(t, string(test.outputJSON), string(m))
			}
		})
	}
}

func kubeOpenAPIKindWithProps(gvk schema.GroupVersionKind, ref common.ReferenceCallback, props map[string]spec.SchemaProps, deps ...string) common.OpenAPIDefinition {
	required := []string{"kind", "apiVersion", "metadata"}
	if _, ok := props["spec"]; ok {
		required = append(required, "spec")
	}
	kind := common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Type: []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
				},
				Required: required,
			},
		},
		Dependencies: make([]string, 0),
	}
	kind.Dependencies = append(kind.Dependencies, "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta")
	for k, prop := range props {
		kind.Schema.Properties[k] = spec.Schema{
			SchemaProps: prop,
		}
		if prop.Ref.String() != "" {
			kind.Dependencies = append(kind.Dependencies, prop.Ref.String())
		}
	}
	for _, dep := range deps {
		if slices.Contains(kind.Dependencies, dep) {
			continue
		}
		kind.Dependencies = append(kind.Dependencies, dep)
	}
	return kind
}

func kubeOpenAPIList(gvk schema.GroupVersionKind, ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Type: []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"),
						},
					},
					"items": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref(fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)),
									},
								},
							},
						},
					},
				},
				Required: []string{"metadata", "items"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta", fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)},
	}
}

func ptr[T any](in T) *T {
	return &in
}
