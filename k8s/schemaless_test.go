package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	k8sErrors "github.com/grafana/grafana-app-sdk/k8s/errors"
	"github.com/grafana/grafana-app-sdk/resource"
)

var testGroupVersions = []schema.GroupVersion{
	{
		Group:   "g1",
		Version: "v1",
	},
	{
		Group:   "g2",
		Version: "v1",
	},
}

func TestSchemalessClient_Get(t *testing.T) {
	client, server := getSchemalessClientTestSetup(testGroupVersions...)
	defer server.Close()
	id1 := resource.FullIdentifier{
		Namespace: "ns",
		Name:      "testo",
		Group:     testGroupVersions[0].Group,
		Version:   testGroupVersions[0].Version,
		Kind:      "foo",
	}
	id2 := resource.FullIdentifier{
		Namespace: "ns",
		Name:      "testo",
		Group:     testGroupVersions[1].Group,
		Version:   testGroupVersions[1].Version,
		Kind:      "bar",
		Plural:    "unexpected",
	}
	ctx := context.TODO()

	t.Run("nil into", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil into")
		}

		err := client.Get(ctx, id1, nil)
		assert.Equal(t, fmt.Errorf("into cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		into := resource.SimpleObject[any]{}
		err := client.Get(ctx, id2, &into)
		assert.Equal(t, resource.SimpleObject[any]{}, into)
		require.NotNil(t, err)
		cast, ok := err.(*k8sErrors.ServerResponseError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, cast.StatusCode())
	})

	t.Run("success, id1", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id1.Namespace, fmt.Sprintf("%ss", id1.Kind), id1.Name), r.URL.Path)
		}

		into := resource.SimpleObject[testSpec]{}
		err := client.Get(ctx, id1, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.StaticMetadata(), into.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), into.CommonMetadata())
		assert.Equal(t, responseObj.Spec, into.Spec)
		assert.Equal(t, responseObj.Subresources(), into.Subresources())
	})

	t.Run("success, id2", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id2.Namespace, id2.Plural, id2.Name), r.URL.Path)
		}

		into := resource.SimpleObject[testSpec]{}
		err := client.Get(ctx, id2, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.StaticMetadata(), into.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), into.CommonMetadata())
		assert.Equal(t, responseObj.Spec, into.Spec)
		assert.Equal(t, responseObj.Subresources(), into.Subresources())
	})
}

func TestSchemalessClient_Create(t *testing.T) {
	client, server := getSchemalessClientTestSetup(testGroupVersions...)
	defer server.Close()
	id := resource.FullIdentifier{
		Namespace: "ns",
		Name:      "testo",
		Group:     testGroupVersions[0].Group,
		Version:   testGroupVersions[0].Version,
		Kind:      "foo",
	}
	ctx := context.TODO()

	t.Run("nil obj", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil obj")
		}

		err := client.Create(ctx, id, nil, resource.CreateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("obj cannot be nil"), err)
	})

	t.Run("nil into", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil into")
		}

		err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("into cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{}, &resource.SimpleObject[any]{})
		require.NotNil(t, err)
		cast, ok := err.(*k8sErrors.ServerResponseError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, cast.StatusCode())
	})

	t.Run("success", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.Nil(t, err)
			posted := submittedObj{}
			require.Nil(t, json.Unmarshal(body, &posted))
			// id info should have overwritten StaticMeta in object:
			assert.Equal(t, id.Namespace, posted.ObjectMetadata.Namespace)
			assert.Equal(t, id.Name, posted.ObjectMetadata.Name)
			assert.Equal(t, responseObj.Spec, posted.Spec)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind)), r.URL.Path)
		}

		into := resource.SimpleObject[testSpec]{}
		err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.StaticMetadata(), into.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), into.CommonMetadata())
		assert.Equal(t, responseObj.SpecObject(), into.SpecObject())
		assert.Equal(t, responseObj.Subresources(), into.Subresources())
	})
}

func TestSchemalessClient_Update(t *testing.T) {
	client, server := getSchemalessClientTestSetup(testGroupVersions...)
	defer server.Close()
	id := resource.FullIdentifier{
		Namespace: "ns",
		Name:      "testo",
		Group:     testGroupVersions[0].Group,
		Version:   testGroupVersions[0].Version,
		Kind:      "foo",
	}
	ctx := context.TODO()

	t.Run("nil obj", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil obj")
		}

		err := client.Update(ctx, id, nil, resource.UpdateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("obj cannot be nil"), err)
	})

	t.Run("nil into", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil into")
		}

		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("into cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{}, getTestObject())
		require.NotNil(t, err)
		cast, ok := err.(*k8sErrors.ServerResponseError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, cast.StatusCode())
	})

	t.Run("success, explicit RV", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.Nil(t, err)
			posted := submittedObj{}
			assert.Nil(t, json.Unmarshal(body, &posted))
			// id info should have overwritten StaticMeta in object:
			assert.Equal(t, id.Namespace, posted.ObjectMetadata.Namespace)
			assert.Equal(t, id.Name, posted.ObjectMetadata.Name)
			assert.Equal(t, responseObj.Spec, posted.Spec)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
		}

		into := resource.SimpleObject[testSpec]{}
		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{
			ResourceVersion: responseObj.CommonMeta.ResourceVersion,
		}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.StaticMetadata(), into.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), into.CommonMetadata())
		assert.Equal(t, responseObj.SpecObject(), into.SpecObject())
		assert.Equal(t, responseObj.Subresources(), into.Subresources())
	})

	t.Run("success, no RV", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				// Get to get RV
				writer.Write(responseBytes)
				return
			}
			assert.Equal(t, http.MethodPut, r.Method)
			body, err := io.ReadAll(r.Body)
			require.Nil(t, err)
			posted := submittedObj{}
			assert.Nil(t, json.Unmarshal(body, &posted))
			// id info should have overwritten StaticMeta in object:
			assert.Equal(t, id.Namespace, posted.ObjectMetadata.Namespace)
			assert.Equal(t, id.Name, posted.ObjectMetadata.Name)
			assert.Equal(t, responseObj.Spec, posted.Spec)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
		}

		into := resource.SimpleObject[testSpec]{}
		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.StaticMetadata(), into.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), into.CommonMetadata())
		assert.Equal(t, responseObj.SpecObject(), into.SpecObject())
		assert.Equal(t, responseObj.Subresources(), into.Subresources())
	})

	t.Run("success, subresource", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			body, err := io.ReadAll(r.Body)
			require.Nil(t, err)
			posted := submittedObj{}
			assert.Nil(t, json.Unmarshal(body, &posted))
			// id info should have overwritten StaticMeta in object:
			assert.Equal(t, id.Namespace, posted.ObjectMetadata.Namespace)
			assert.Equal(t, id.Name, posted.ObjectMetadata.Name)
			assert.Equal(t, responseObj.Spec, posted.Spec)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s/status", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
		}

		into := resource.SimpleObject[testSpec]{}
		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{
			ResourceVersion: responseObj.CommonMeta.ResourceVersion,
			Subresource:     "status",
		}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.StaticMetadata(), into.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), into.CommonMetadata())
		assert.Equal(t, responseObj.SpecObject(), into.SpecObject())
		assert.Equal(t, responseObj.Subresources(), into.Subresources())
	})
}

func TestSchemalessClient_Delete(t *testing.T) {
	client, server := getSchemalessClientTestSetup(testGroupVersions...)
	defer server.Close()
	id := resource.FullIdentifier{
		Namespace: "ns",
		Name:      "testo",
		Group:     testGroupVersions[0].Group,
		Version:   testGroupVersions[0].Version,
		Kind:      "foo",
	}
	ctx := context.TODO()

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		err := client.Delete(ctx, id)
		require.NotNil(t, err)
		cast, ok := err.(*k8sErrors.ServerResponseError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, cast.StatusCode())
	})

	t.Run("success", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
		}

		err := client.Delete(ctx, id)
		assert.Nil(t, err)
	})
}

func TestSchemalessClient_List(t *testing.T) {
	client, server := getSchemalessClientTestSetup(testGroupVersions...)
	defer server.Close()
	id := resource.FullIdentifier{
		Namespace: "ns",
		Name:      "testo",
		Group:     testGroupVersions[0].Group,
		Version:   testGroupVersions[0].Version,
		Kind:      "foo",
	}
	ctx := context.TODO()
	listResp := testList{
		TypeMeta: metav1.TypeMeta{
			Kind: responseObj.StaticMeta.Kind,
		},
		Metadata: metav1.ListMeta{},
		Items: []submittedObj{{
			TypeMeta:       k8sResponseObject.TypeMeta,
			ObjectMetadata: k8sResponseObject.ObjectMeta,
			Spec:           responseObj.Spec,
		}},
	}

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		into := resource.SimpleList[*resource.SimpleObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{}, &into, &resource.SimpleObject[testSpec]{})
		require.NotNil(t, err)
		cast, ok := err.(*k8sErrors.ServerResponseError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, cast.StatusCode())
	})

	t.Run("success, no options", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			listBytes, err := json.Marshal(listResp)
			assert.Nil(t, err)
			writer.Write(listBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind)), r.URL.Path)
		}

		into := resource.SimpleList[*resource.SimpleObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{}, &into, &resource.SimpleObject[testSpec]{})
		assert.Nil(t, err)
		assert.Len(t, into.ListItems(), 1)
		item, ok := into.ListItems()[0].(*resource.SimpleObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.StaticMetadata(), item.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), item.CommonMetadata())
		assert.Equal(t, responseObj.SpecObject(), item.SpecObject())
		assert.Equal(t, responseObj.Subresources(), item.Subresources())
	})

	t.Run("success, with filters", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			// Check for filter params
			assert.Equal(t, "a,b", r.URL.Query().Get("labelSelector"))
			listBytes, err := json.Marshal(listResp)
			assert.Nil(t, err)
			writer.Write(listBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind)), r.URL.Path)
		}

		into := resource.SimpleList[*resource.SimpleObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{
			LabelFilters: []string{"a", "b"},
		}, &into, &resource.SimpleObject[testSpec]{})
		assert.Nil(t, err)
		assert.Len(t, into.ListItems(), 1)
		item, ok := into.ListItems()[0].(*resource.SimpleObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.StaticMetadata(), item.StaticMetadata())
		assert.Equal(t, responseObj.CommonMetadata(), item.CommonMetadata())
		assert.Equal(t, responseObj.SpecObject(), item.SpecObject())
		assert.Equal(t, responseObj.Subresources(), item.Subresources())
	})
}

func getSchemalessClientTestSetup(gvs ...schema.GroupVersion) (*SchemalessClient, *testServer) {
	s := testServer{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if s.responseFunc != nil {
			s.responseFunc(writer, request)
		}
	}))
	s.Server = server

	client := NewSchemalessClient(rest.Config{}, ClientConfig{CustomMetadataIsAnyType: false})

	for _, gv := range gvs {
		client.clients[gv.Identifier()] = &groupVersionClient{
			client: getMockClient(server.URL, gv.Group, gv.Version),
		}
	}
	return client, &s
}
