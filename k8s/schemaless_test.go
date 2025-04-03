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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

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

		into := resource.TypedSpecObject[any]{}
		err := client.Get(ctx, id2, &into)
		assert.Equal(t, resource.TypedSpecObject[any]{}, into)
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})

	t.Run("success, id1", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id1.Namespace, fmt.Sprintf("%ss", id1.Kind), id1.Name), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.Get(ctx, id1, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.Spec, into.Spec)
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
	})

	t.Run("success, id2", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id2.Namespace, id2.Plural, id2.Name), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.Get(ctx, id2, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.Spec, into.Spec)
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
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

		err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{}, &resource.TypedSpecObject[any]{})
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
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

		into := resource.TypedSpecObject[testSpec]{}
		err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), into.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
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
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
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

		into := resource.TypedSpecObject[testSpec]{}
		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{
			ResourceVersion: responseObj.GetCommonMetadata().ResourceVersion,
		}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), into.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
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

		into := resource.TypedSpecObject[testSpec]{}
		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), into.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
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

		into := resource.TypedSpecObject[testSpec]{}
		err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{
			ResourceVersion: responseObj.GetCommonMetadata().ResourceVersion,
			Subresource:     "status",
		}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), into.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
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

		err := client.Delete(ctx, id, resource.DeleteOptions{})
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})

	t.Run("success", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
		}

		err := client.Delete(ctx, id, resource.DeleteOptions{})
		assert.Nil(t, err)
	})

	t.Run("propagationPolicy", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
			assert.Equal(t, string(resource.DeleteOptionsPropagationPolicyForeground), r.URL.Query().Get("propagationPolicy"))
		}

		err := client.Delete(ctx, id, resource.DeleteOptions{
			PropagationPolicy: resource.DeleteOptionsPropagationPolicyForeground,
		})
		assert.Nil(t, err)
	})

	t.Run("preconditions", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind), id.Name), r.URL.Path)
			assert.Equal(t, "123", r.URL.Query().Get("preconditions.resourceVersion"))
			assert.Equal(t, "abc", r.URL.Query().Get("preconditions.uid"))
		}

		err := client.Delete(ctx, id, resource.DeleteOptions{
			Preconditions: resource.DeleteOptionsPreconditions{
				ResourceVersion: "123",
				UID:             "abc",
			},
		})
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
			Kind: responseObj.GetStaticMetadata().Kind,
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

		into := resource.TypedList[*resource.TypedSpecObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{}, &into, &resource.TypedSpecObject[testSpec]{})
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
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

		into := resource.TypedList[*resource.TypedSpecObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{}, &into, &resource.TypedSpecObject[testSpec]{})
		assert.Nil(t, err)
		assert.Len(t, into.GetItems(), 1)
		item, ok := into.GetItems()[0].(*resource.TypedSpecObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.GetStaticMetadata(), item.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), item.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), item.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), item.GetSubresources())
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

		into := resource.TypedList[*resource.TypedSpecObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{
			LabelFilters: []string{"a", "b"},
		}, &into, &resource.TypedSpecObject[testSpec]{})
		assert.Nil(t, err)
		assert.Len(t, into.GetItems(), 1)
		item, ok := into.GetItems()[0].(*resource.TypedSpecObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.GetStaticMetadata(), item.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), item.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), item.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), item.GetSubresources())
	})

	t.Run("success, with fieldSelectors", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			// Check for filter params
			assert.Equal(t, "a,b", r.URL.Query().Get("fieldSelector"))
			listBytes, err := json.Marshal(listResp)
			assert.Nil(t, err)
			writer.Write(listBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", id.Namespace, fmt.Sprintf("%ss", id.Kind)), r.URL.Path)
		}

		into := resource.TypedList[*resource.TypedSpecObject[testSpec]]{}
		err := client.List(ctx, id, resource.ListOptions{
			FieldSelectors: []string{"a", "b"},
		}, &into, &resource.TypedSpecObject[testSpec]{})
		assert.Nil(t, err)
		assert.Len(t, into.GetItems(), 1)
		item, ok := into.GetItems()[0].(*resource.TypedSpecObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.GetStaticMetadata(), item.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), item.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), item.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), item.GetSubresources())
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
