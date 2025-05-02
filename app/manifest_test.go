package app

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

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
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
		},
		err: nil,
	}, {
		name:   "spec",
		schema: []byte(`{"spec":{"type":"object","properties":{"foo":{"type":"string"}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
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
		err: nil,
	}, {
		name:   "dependencies",
		schema: []byte(`{"#foo":{"type":"object","properties":{"foobar":{"type":"string"}}},"spec":{"type":"object","properties":{"foo":{"type":"string"},"bar":{"$ref":"#/components/schemas/#foo"}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.#foo": common.OpenAPIDefinition{
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
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						Properties: map[string]spec.Schema{
							"foo": {
								SchemaProps: spec.SchemaProps{
									Type: []string{"string"},
								},
							},
							"bar": {
								SchemaProps: spec.SchemaProps{
									Ref: ref("test.grafana.app/v1.#foo"),
								},
							},
						},
					},
				},
				Dependencies: []string{"test.grafana.app/v1.#foo"},
			},
		},
	}, {
		name:   "additional subresources",
		schema: []byte(`{"status":{"type":"object","properties":{"foobar":{"type":"string"}}},"spec":{"type":"object","properties":{"foo":{"type":"string"}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec", "status"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
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
			"test.grafana.app/v1.status": common.OpenAPIDefinition{
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
		name:   "freeform additionalProperties",
		schema: []byte(`{"spec":{"type":"object","additionalProperties":true}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						AdditionalProperties: &spec.SchemaOrBool{
							Allows: true,
						},
					},
				},
			},
		},
	}, {
		name:   "defined additionalProperties",
		schema: []byte(`{"spec":{"type":"object","additionalProperties":{"type":"object","properties":{"foo":{"type":"string"}}}}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
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
				},
			},
		},
	}, {
		name:   "defined additionalProperties",
		schema: []byte(`{"spec":{"type":"object","x-kubernetes-preserve-unknown-fields":true}}`),
		gvk:    gvk,
		ref:    ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						AdditionalProperties: &spec.SchemaOrBool{
							Allows: true,
						},
					},
				},
			},
		},
	}, {
		name: "full complex schema",
		schema: []byte(`{
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
}`),
		gvk: gvk,
		ref: ref,
		want: map[string]common.OpenAPIDefinition{
			"test.grafana.app/v1.Foo":     kubeOpenAPIKindWithProps(gvk, ref, []string{"spec", "status"}),
			"test.grafana.app/v1.FooList": kubeOpenAPIList(gvk, ref),
			"test.grafana.app/v1.#foo": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						AdditionalProperties: &spec.SchemaOrBool{
							Allows: true,
						},
					},
				},
			},
			"test.grafana.app/v1.#bar": common.OpenAPIDefinition{
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
			"test.grafana.app/v1.#foobar": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"string"},
					},
				},
			},
			"test.grafana.app/v1.spec": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						Properties: map[string]spec.Schema{
							"all": {
								SchemaProps: spec.SchemaProps{
									AllOf: []spec.Schema{{
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#foo"),
										},
									}, {
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#bar"),
										},
									}},
								},
							},
							"any": {
								SchemaProps: spec.SchemaProps{
									AnyOf: []spec.Schema{{
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#foo"),
										},
									}, {
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#bar"),
										},
									}, {
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#foobar"),
										},
									}},
								},
							},
							"one": {
								SchemaProps: spec.SchemaProps{
									OneOf: []spec.Schema{{
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#foo"),
										},
									}, {
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#foobar"),
										},
									}},
								},
							},
							"no": {
								SchemaProps: spec.SchemaProps{
									Not: &spec.Schema{
										SchemaProps: spec.SchemaProps{
											Ref: ref("test.grafana.app/v1.#foo"),
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
												Ref: ref("test.grafana.app/v1.#foo"),
											},
										},
									},
								},
							},
						},
					},
				},
				Dependencies: []string{"test.grafana.app/v1.#bar", "test.grafana.app/v1.#foo", "test.grafana.app/v1.#foobar"},
			},
			"test.grafana.app/v1.status": common.OpenAPIDefinition{
				Schema: spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: spec.StringOrArray{"object"},
						AdditionalProperties: &spec.SchemaOrBool{
							Allows: true,
						},
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			vs := VersionSchema{}
			require.Nil(t, json.Unmarshal(test.schema, &vs))
			res, err := vs.AsKubeOpenAPI(test.gvk, test.ref)
			if test.err != nil {
				assert.Equal(t, test.err, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.want, res)
			}
		})
	}
}

func kubeOpenAPIKindWithProps(gvk schema.GroupVersionKind, ref common.ReferenceCallback, props []string) common.OpenAPIDefinition {
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
			},
		},
		Dependencies: make([]string, 0),
	}
	for _, prop := range props {
		kind.Dependencies = append(kind.Dependencies, fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, prop))
		kind.Schema.Properties[prop] = spec.Schema{
			SchemaProps: spec.SchemaProps{
				Default: map[string]interface{}{},
				Ref:     ref(fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, prop)),
			},
		}
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
