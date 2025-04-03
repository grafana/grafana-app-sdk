package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestResourceManager_RegisterSchema(t *testing.T) {
	manager, server := getTestManagerAndServer()
	defer server.Close()
	ctx := context.TODO()

	t.Run("error on get call", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}
		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{})
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})

	t.Run("exists, version exists, error on conflict", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			assert.Equal(t, http.MethodGet, request.Method)
			b, err := json.Marshal(CustomResourceDefinition{
				Spec: CustomResourceDefinitionSpec{
					Versions: []CustomResourceDefinitionSpecVersion{
						{
							Name: testSchema.Version(),
						},
					},
				},
			})
			require.Nil(t, err)
			writer.Write(b)
		}

		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{})
		assert.Equal(t, fmt.Errorf("schema with identical kind, group, and version already registered"), err)
	})

	t.Run("exists, version exists, no error on conflict", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			assert.Equal(t, http.MethodGet, request.Method)
			b, err := json.Marshal(CustomResourceDefinition{
				Spec: CustomResourceDefinitionSpec{
					Versions: []CustomResourceDefinitionSpecVersion{
						{
							Name: testSchema.Version(),
						},
					},
				},
			})
			require.Nil(t, err)
			writer.Write(b)
		}

		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{
			NoErrorOnConflict: true, // Return nil on conflict
		})
		assert.Nil(t, err)
	})

	t.Run("exists, no version, error on conflict", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				b, err := json.Marshal(CustomResourceDefinition{
					Spec: CustomResourceDefinitionSpec{
						Versions: []CustomResourceDefinitionSpecVersion{
							{
								Name: testSchema.Version() + "_no",
							},
						},
					},
				})
				require.Nil(t, err)
				writer.Write(b)
				return
			}
			assert.Equal(t, http.MethodPut, request.Method)
			// Check that the new version is there
			body, err := io.ReadAll(request.Body)
			assert.Nil(t, err)
			um := CustomResourceDefinition{}
			assert.Nil(t, json.Unmarshal(body, &um))
			assert.Len(t, um.Spec.Versions, 2)
			assert.Equal(t, testSchema.Version(), um.Spec.Versions[1].Name)
		}

		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{})
		assert.Nil(t, err)
	})

	t.Run("exists, version exists, update on conflict", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				b, err := json.Marshal(CustomResourceDefinition{
					Spec: CustomResourceDefinitionSpec{
						Versions: []CustomResourceDefinitionSpecVersion{
							{
								Name: testSchema.Version(),
							},
						},
					},
				})
				require.Nil(t, err)
				writer.Write(b)
				return
			}
			assert.Equal(t, http.MethodPut, request.Method)
			// Check that the version was updated
			body, err := io.ReadAll(request.Body)
			assert.Nil(t, err)
			um := CustomResourceDefinition{}
			assert.Nil(t, json.Unmarshal(body, &um))
			assert.Len(t, um.Spec.Versions, 1)
			assert.Equal(t, toVersion(testSchema).Schema, um.Spec.Versions[0].Schema)
		}

		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{
			UpdateOnConflict: true,
		})
		assert.Nil(t, err)
	})

	t.Run("doesn't exist, success", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			assert.Equal(t, http.MethodPost, request.Method)
			// Check that the body is right
			body, err := io.ReadAll(request.Body)
			assert.Nil(t, err)
			um := CustomResourceDefinition{}
			assert.Nil(t, json.Unmarshal(body, &um))
			assert.Len(t, um.Spec.Versions, 1)
			assert.Equal(t, toVersion(testSchema).Schema, um.Spec.Versions[0].Schema)
		}

		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{
			UpdateOnConflict: true,
		})
		assert.Nil(t, err)
	})

	t.Run("error on create", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.WriteHeader(http.StatusBadRequest)
		}
		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{})
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})

	t.Run("error on update", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodPut {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			b, err := json.Marshal(CustomResourceDefinition{
				Spec: CustomResourceDefinitionSpec{
					Versions: []CustomResourceDefinitionSpecVersion{
						{
							Name: testSchema.Version(),
						},
					},
				},
			})
			require.Nil(t, err)
			writer.Write(b)
		}
		err := manager.RegisterSchema(ctx, testSchema, resource.RegisterSchemaOptions{
			UpdateOnConflict: true,
		})
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})
}

func TestResourceManager_WaitForAvailability(t *testing.T) {
	manager, server := getTestManagerAndServer()
	defer server.Close()

	t.Run("unknown error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}
		ctx, _ := context.WithTimeout(context.TODO(), time.Second)
		err := manager.WaitForAvailability(ctx, testSchema)
		assert.NotNil(t, err)
	})

	t.Run("timeout reached", func(t *testing.T) {
		requestCount := 0
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			requestCount++
			writer.WriteHeader(http.StatusNotFound)
		}
		ctx, _ := context.WithTimeout(context.TODO(), time.Second*2)
		err := manager.WaitForAvailability(ctx, testSchema)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		assert.GreaterOrEqual(t, 2, requestCount)
	})

	t.Run("success", func(t *testing.T) {
		requestCount := 0
		server.responseFunc = func(writer http.ResponseWriter, request *http.Request) {
			if requestCount > 0 {
				def, _ := json.Marshal(CustomResourceDefinition{})
				writer.Write(def)
				writer.WriteHeader(http.StatusOK)
			} else {
				writer.WriteHeader(http.StatusNotFound)
			}
			requestCount++
		}
		ctx, _ := context.WithTimeout(context.TODO(), time.Second*5)
		err := manager.WaitForAvailability(ctx, testSchema)
		assert.Nil(t, err)
		assert.GreaterOrEqual(t, 2, requestCount)
	})
}

func getTestManagerAndServer() (*ResourceManager, *testServer) {
	s := testServer{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if s.responseFunc != nil {
			s.responseFunc(writer, request)
		}
	}))
	s.Server = server
	client := getMockClient(server.URL, "customresourcedefinitions", "v1")
	manager := ResourceManager{
		client: client,
	}
	return &manager, &s
}

/**
Unexported methods tests (for thoroughness)
*/

func TestToOpenAPIV3(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		type L3 struct {
			Final int `json:"final"`
		}
		type L2 struct {
			L3
			Inner string `json:"inner"`
		}
		type L1 struct {
			l2 *L2 `json:"next"`
		}
		res := toOpenAPIV3(reflect.TypeOf(&L1{}))
		assert.Equal(t, res, map[string]any{
			"next": map[string]any{
				"properties": map[string]any{
					"inner": map[string]any{
						"type": "string",
					},
					"final": map[string]any{
						"type": "integer",
					},
				},
				"type": "object",
			},
		})
	})

	t.Run("embedded map", func(t *testing.T) {
		type M map[string]any
		type Spec struct {
			M
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{
			"x-kubernetes-preserve-unknown-fields": true,
		})
	})

	t.Run("map key", func(t *testing.T) {
		type Spec struct {
			M map[string]any `json:"map"`
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{
			"map": map[string]any{
				"type":                                 "object",
				"x-kubernetes-preserve-unknown-fields": true,
			},
		})
	})

	t.Run("int key", func(t *testing.T) {
		type Spec struct {
			I   int   `json:"i"`
			I16 int16 `json:"i16"`
			I32 int32 `json:"i32"`
			I64 int64 `json:"i64"`
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{
			"i": map[string]any{
				"type": "integer",
			},
			"i16": map[string]any{
				"type": "integer",
			},
			"i32": map[string]any{
				"type": "integer",
			},
			"i64": map[string]any{
				"type": "integer",
			},
		})
	})

	t.Run("float key", func(t *testing.T) {
		type Spec struct {
			F32 float32 `json:"f32"`
			F64 float64 `json:"f64"`
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{
			"f32": map[string]any{
				"type": "number",
			},
			"f64": map[string]any{
				"type": "number",
			},
		})
	})

	t.Run("bool key", func(t *testing.T) {
		type Spec struct {
			B bool `json:"b"`
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{
			"b": map[string]any{
				"type": "boolean",
			},
		})
	})

	t.Run("array key", func(t *testing.T) {
		type Element struct {
			Foo string `json:"foo"`
		}
		type Spec struct {
			S   []string      `json:"slice"`
			A   [1]int        `json:"array"`
			OS  []Element     `json:"objslice"`
			IS  []*int32      `json:"islice"`
			FS  []float64     `json:"fslice"`
			BS  []bool        `json:"bslice"`
			Any []interface{} `json:"any"`
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{
			"slice": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"array": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "integer",
				},
			},
			"objslice": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"foo": map[string]any{
							"type": "string",
						},
					},
				},
			},
			"islice": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "integer",
				},
			},
			"fslice": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "number",
				},
			},
			"bslice": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "boolean",
				},
			},
			"any": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                                 "object",
					"x-kubernetes-preserve-unknown-fields": true,
				},
			},
		})
	})

	t.Run("unhandlable key", func(t *testing.T) {
		type Spec struct {
			Ch chan string
		}
		res := toOpenAPIV3(reflect.TypeOf(Spec{}))
		assert.Equal(t, res, map[string]any{})
	})
}

func TestGetFieldKey(t *testing.T) {
	t.Run("no JSON tag", func(t *testing.T) {
		type Foo struct {
			Bar string
		}
		field := reflect.TypeOf(Foo{}).Field(0)
		assert.Equal(t, "Bar", getFieldKey(&field))
	})

	t.Run("JSON tag", func(t *testing.T) {
		type Foo struct {
			Bar string `json:"bar"`
		}
		field := reflect.TypeOf(Foo{}).Field(0)
		assert.Equal(t, "bar", getFieldKey(&field))
	})

	t.Run("JSON tag with extras", func(t *testing.T) {
		type Foo struct {
			Bar string `json:"bar,omitempty"`
		}
		field := reflect.TypeOf(Foo{}).Field(0)
		assert.Equal(t, "bar", getFieldKey(&field))
	})
}
