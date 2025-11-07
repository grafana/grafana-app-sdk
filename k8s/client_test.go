package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/grafana/grafana-app-sdk/resource"
)

var (
	testSchema = resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[testSpec]{}, &resource.TypedList[*resource.TypedSpecObject[testSpec]]{}, resource.WithKind("test"))
	testKind   = resource.Kind{
		Schema: testSchema,
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	responseObj       = getTestObject()
	k8sResponseObject = struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata"`
		Spec              testSpec `json:"spec"`
	}{
		TypeMeta: metav1.TypeMeta{
			Kind: responseObj.GetStaticMetadata().Kind,
			APIVersion: schema.GroupVersion{
				Group:   responseObj.GetStaticMetadata().Group,
				Version: responseObj.GetStaticMetadata().Version,
			}.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            responseObj.GetStaticMetadata().Name,
			Namespace:       responseObj.GetStaticMetadata().Namespace,
			ResourceVersion: responseObj.GetCommonMetadata().ResourceVersion,
			Labels:          responseObj.GetCommonMetadata().Labels,
		},
		Spec: responseObj.Spec,
	}
	responseBytes, _ = json.Marshal(k8sResponseObject)
)

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()
	// Check the KubeConfigProvider's logic
	t.Run("KubeConfigProvider doesn't modify APIPath when already set", func(t *testing.T) {
		existingPath := "/a/path"
		rcfg := rest.Config{
			APIPath: existingPath,
		}
		provided := config.KubeConfigProvider(testKind, rcfg)
		assert.Equal(t, rcfg, provided)
	})
	t.Run("KubeConfigProvider sets APIPath to /apis for custom kinds", func(t *testing.T) {
		rcfg := rest.Config{}
		provided := config.KubeConfigProvider(testKind, rcfg)
		assert.NotEqual(t, rcfg, provided)
		assert.Equal(t, "/apis", provided.APIPath)
	})
	t.Run("KubeConfigProvider sets APIPath to /apis for kinds with an empty group (legacy k8s kinds)", func(t *testing.T) {
		rcfg := rest.Config{}
		provided := config.KubeConfigProvider(resource.Kind{
			Schema: resource.NewSimpleSchema("", "v1", &resource.UntypedObject{}, &resource.UntypedList{}),
		}, rcfg)
		assert.NotEqual(t, rcfg, provided)
		assert.Equal(t, "/api", provided.APIPath)
	})
}

func TestClient_Get(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
	}
	ctx := context.TODO()

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		resp, err := client.Get(ctx, id)
		assert.Nil(t, resp)
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})

	t.Run("success", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		resp, err := client.Get(ctx, id)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), resp.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), resp.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), resp.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), resp.GetSubresources())
	})
}

func TestClient_GetInto(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
	}
	ctx := context.TODO()

	t.Run("nil into", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil into")
		}

		err := client.GetInto(ctx, id, nil)
		assert.Equal(t, fmt.Errorf("into cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		into := resource.TypedSpecObject[any]{}
		err := client.GetInto(ctx, id, &into)
		assert.Equal(t, resource.TypedSpecObject[any]{}, into)
		require.NotNil(t, err)
		cast, ok := err.(apierrors.APIStatus)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, int(cast.Status().Code))
	})

	t.Run("success", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.GetInto(ctx, id, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.Spec, into.Spec)
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
	})
}

func TestClient_Create(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
	}
	ctx := context.TODO()

	t.Run("nil obj", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil obj")
		}

		resp, err := client.Create(ctx, id, nil, resource.CreateOptions{})
		assert.Nil(t, resp)
		assert.Equal(t, fmt.Errorf("obj cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		resp, err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{})
		assert.Nil(t, resp)
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", id.Namespace, testSchema.Plural()), r.URL.Path)
		}

		resp, err := client.Create(ctx, id, getTestObject(), resource.CreateOptions{})
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), resp.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), resp.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), resp.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), resp.GetSubresources())
	})
}

func TestClient_CreateInto(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
	}
	ctx := context.TODO()

	t.Run("nil obj", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil obj")
		}

		err := client.CreateInto(ctx, id, nil, resource.CreateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("obj cannot be nil"), err)
	})

	t.Run("nil into", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil into")
		}

		err := client.CreateInto(ctx, id, getTestObject(), resource.CreateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("into cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		err := client.CreateInto(ctx, id, getTestObject(), resource.CreateOptions{}, &resource.TypedSpecObject[any]{})
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", id.Namespace, testSchema.Plural()), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.CreateInto(ctx, id, getTestObject(), resource.CreateOptions{}, &into)
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), into.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), into.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), into.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), into.GetSubresources())
	})
}

func TestClient_Update(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
	}
	ctx := context.TODO()

	t.Run("nil obj", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil obj")
		}

		resp, err := client.Update(ctx, id, nil, resource.UpdateOptions{})
		assert.Nil(t, resp)
		assert.Equal(t, fmt.Errorf("obj cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		resp, err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{})
		assert.Nil(t, resp)
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		resp, err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{
			ResourceVersion: responseObj.GetCommonMetadata().ResourceVersion,
		})
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), resp.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), resp.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), resp.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), resp.GetSubresources())
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		resp, err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{})
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), resp.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), resp.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), resp.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), resp.GetSubresources())
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s/status", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		resp, err := client.Update(ctx, id, getTestObject(), resource.UpdateOptions{
			ResourceVersion: responseObj.GetCommonMetadata().ResourceVersion,
			Subresource:     "status",
		})
		assert.Nil(t, err)
		assert.Equal(t, responseObj.GetStaticMetadata(), resp.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), resp.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), resp.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), resp.GetSubresources())
	})
}

func TestClient_UpdateInto(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
	}
	ctx := context.TODO()

	t.Run("nil obj", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil obj")
		}

		err := client.UpdateInto(ctx, id, nil, resource.UpdateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("obj cannot be nil"), err)
	})

	t.Run("nil into", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Fail(t, "HTTP request should not be made for nil into")
		}

		err := client.UpdateInto(ctx, id, getTestObject(), resource.UpdateOptions{}, nil)
		assert.Equal(t, fmt.Errorf("into cannot be nil"), err)
	})

	t.Run("http error", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			writer.WriteHeader(http.StatusBadRequest)
		}

		err := client.UpdateInto(ctx, id, getTestObject(), resource.UpdateOptions{}, getTestObject())
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.UpdateInto(ctx, id, getTestObject(), resource.UpdateOptions{
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.UpdateInto(ctx, id, getTestObject(), resource.UpdateOptions{}, &into)
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s/status", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		into := resource.TypedSpecObject[testSpec]{}
		err := client.UpdateInto(ctx, id, getTestObject(), resource.UpdateOptions{
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

func TestClient_Delete(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	id := resource.Identifier{
		Namespace: "ns",
		Name:      "testo",
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
		}

		err := client.Delete(ctx, id, resource.DeleteOptions{})
		assert.Nil(t, err)
	})

	t.Run("propagationPolicy", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			writer.Write(responseBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s/%s", id.Namespace, testSchema.Plural(), id.Name), r.URL.Path)
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

func TestClient_List(t *testing.T) {
	client, server := getClientTestSetup(testKind)
	defer server.Close()
	ctx := context.TODO()
	ns := "ns"
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

		list, err := client.List(ctx, ns, resource.ListOptions{})
		assert.Nil(t, list)
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", ns, testSchema.Plural()), r.URL.Path)
		}

		list, err := client.List(ctx, ns, resource.ListOptions{})
		assert.Nil(t, err)
		assert.NotNil(t, list)
		assert.Len(t, list.GetItems(), 1)
		item, ok := list.GetItems()[0].(*resource.TypedSpecObject[testSpec])
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
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", ns, testSchema.Plural()), r.URL.Path)
		}

		list, err := client.List(ctx, ns, resource.ListOptions{
			LabelFilters: []string{"a", "b"},
		})
		assert.Nil(t, err)
		assert.NotNil(t, list)
		assert.Len(t, list.GetItems(), 1)
		item, ok := list.GetItems()[0].(*resource.TypedSpecObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.GetStaticMetadata(), item.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), item.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), item.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), item.GetSubresources())
	})
	t.Run("success, with field selectors", func(t *testing.T) {
		server.responseFunc = func(writer http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			// Check for filter params
			assert.Equal(t, "a,b", r.URL.Query().Get("fieldSelector"))
			listBytes, err := json.Marshal(listResp)
			assert.Nil(t, err)
			writer.Write(listBytes)
			writer.WriteHeader(http.StatusOK)
			assert.Equal(t, fmt.Sprintf("/namespaces/%s/%s", ns, testSchema.Plural()), r.URL.Path)
		}

		list, err := client.List(ctx, ns, resource.ListOptions{
			FieldSelectors: []string{"a", "b"},
		})
		assert.Nil(t, err)
		assert.NotNil(t, list)
		assert.Len(t, list.GetItems(), 1)
		item, ok := list.GetItems()[0].(*resource.TypedSpecObject[testSpec])
		assert.True(t, ok)
		assert.Equal(t, responseObj.GetStaticMetadata(), item.GetStaticMetadata())
		assert.Equal(t, responseObj.GetCommonMetadata(), item.GetCommonMetadata())
		assert.Equal(t, responseObj.GetSpec(), item.GetSpec())
		assert.Equal(t, responseObj.GetSubresources(), item.GetSubresources())
	})
}

func TestClient_Client(t *testing.T) {
	restClient := getMockClient("http://localhost", testSchema.Group(), testSchema.Version())
	client := Client{
		client: &groupVersionClient{
			client: restClient,
		},
		schema: testSchema,
	}
	assert.Equal(t, restClient, client.RESTClient())
}

func getTestObject() *resource.TypedSpecObject[testSpec] {
	return &resource.TypedSpecObject[testSpec]{
		TypeMeta: metav1.TypeMeta{
			Kind:       testSchema.Kind(),
			APIVersion: fmt.Sprintf("%s/%s", testSchema.Group(), testSchema.Version()),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "namespace",
			Name:            "name",
			ResourceVersion: "rev1",
			Generation:      0,
			Labels: map[string]string{
				"foo":  "bar",
				"test": "value",
			},
		},
		Spec: testSpec{
			Test1: "111",
			Test2: "test",
		},
	}
}

type testList struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ListMeta `json:"metadata"`
	Items           []submittedObj  `json:"items"`
}

type submittedObj struct {
	metav1.TypeMeta `json:",inline"`
	ObjectMetadata  metav1.ObjectMeta `json:"metadata"`
	Spec            testSpec          `json:"spec"`
}

type testSpec struct {
	Test1 string
	Test2 string
}

type testServer struct {
	*httptest.Server
	responseFunc func(http.ResponseWriter, *http.Request)
}

func getClientTestSetup(schema resource.Kind) (*Client, *testServer) {
	s := testServer{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if s.responseFunc != nil {
			s.responseFunc(writer, request)
		}
	}))
	s.Server = server
	client := getMockClient(server.URL, schema.Group(), schema.Version())
	return &Client{
		client: &groupVersionClient{
			client: client,
		},
		schema: schema,
		codec:  schema.Codec(resource.KindEncodingJSON),
	}, &s
}

/*
Everything below here is used for setting up a mock kubernetes client. We can create a mock rest.Interface,
but it has to return a non-interface *rest.Request, which has an embedded http.Client (also not an interface),
making the only testing option actually setting up a test HTTP server for the http.Client to make requests to,
and mock out the appropriate kubernetes responses.
*/

func getMockClient(serverURL, group, version string) *mockRESTClient {
	return &mockRESTClient{
		GetFunc: func() *rest.Request {
			u, _ := url.Parse(serverURL)
			return rest.NewRequestWithClient(u, "", rest.ClientContentConfig{
				GroupVersion: schema.GroupVersion{
					Group:   group,
					Version: version,
				},
				Negotiator: &mockNegotiator{},
			}, &http.Client{}).Verb("GET")
		},
		PostFunc: func() *rest.Request {
			u, _ := url.Parse(serverURL)
			return rest.NewRequestWithClient(u, "", rest.ClientContentConfig{
				GroupVersion: schema.GroupVersion{
					Group:   group,
					Version: version,
				},
				Negotiator: &mockNegotiator{},
			}, &http.Client{}).Verb("POST")
		},
		PatchFunc: func(pt types.PatchType) *rest.Request {
			u, _ := url.Parse(serverURL)
			return rest.NewRequestWithClient(u, "", rest.ClientContentConfig{
				GroupVersion: schema.GroupVersion{
					Group:   group,
					Version: version,
				},
				Negotiator: &mockNegotiator{},
			}, &http.Client{}).Verb("PATCH").Param("patchType", string(pt))
		},
		PutFunc: func() *rest.Request {
			u, _ := url.Parse(serverURL)
			return rest.NewRequestWithClient(u, "", rest.ClientContentConfig{
				GroupVersion: schema.GroupVersion{
					Group:   group,
					Version: version,
				},
				Negotiator: &mockNegotiator{},
			}, &http.Client{}).Verb("PUT")
		},
		DeleteFunc: func() *rest.Request {
			u, _ := url.Parse(serverURL)
			return rest.NewRequestWithClient(u, "", rest.ClientContentConfig{
				GroupVersion: schema.GroupVersion{
					Group:   group,
					Version: version,
				},
				Negotiator: &mockNegotiator{},
			}, &http.Client{}).Verb("DELETE")
		},
	}
}

type decoder struct {
	objects map[string]runtime.Object
	lists   map[string]runtime.Object
}

func (d *decoder) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	type check struct {
		Kind  string        `json:"kind"`
		Items []interface{} `json:"items,omitempty"`
	}
	if into == nil {
		fmt.Println("OH NO")
		c := check{}
		err := json.Unmarshal(data, &c)
		fmt.Println(err)
	}
	err := json.Unmarshal(data, into)
	return into, defaults, err
}

func (d *decoder) Encode(obj runtime.Object, w io.Writer) error {
	b, e := json.Marshal(obj)
	if e != nil {
		return e
	}
	_, e = w.Write(b)
	return e
}

func (d *decoder) Identifier() runtime.Identifier {
	return runtime.Identifier("mock")
}

type framer struct {
}

func (f *framer) NewFrameReader(r io.ReadCloser) io.ReadCloser {
	return r
}
func (f *framer) NewFrameWriter(w io.Writer) io.Writer {
	return w
}

type mockNegotiator struct {
}

func (n *mockNegotiator) Encoder(contentType string, params map[string]string) (runtime.Encoder, error) {
	return &decoder{}, nil
}
func (n *mockNegotiator) Decoder(contentType string, params map[string]string) (runtime.Decoder, error) {
	return &decoder{}, nil
}
func (n *mockNegotiator) StreamDecoder(contentType string, params map[string]string) (runtime.Decoder, runtime.Serializer, runtime.Framer, error) {
	d := &decoder{}
	return d, d, &framer{}, nil
}

type mockRESTClient struct {
	GetRateLimiterFunc func() flowcontrol.RateLimiter
	VerbFunc           func(verb string) *rest.Request
	PostFunc           func() *rest.Request
	PutFunc            func() *rest.Request
	PatchFunc          func(pt types.PatchType) *rest.Request
	GetFunc            func() *rest.Request
	DeleteFunc         func() *rest.Request
	APIVersionFunc     func() schema.GroupVersion
}

func (r *mockRESTClient) GetRateLimiter() flowcontrol.RateLimiter {
	if r.GetRateLimiterFunc != nil {
		return r.GetRateLimiterFunc()
	}
	return nil
}

func (r *mockRESTClient) Verb(verb string) *rest.Request {
	if r.VerbFunc != nil {
		return r.VerbFunc(verb)
	}
	return nil
}
func (r *mockRESTClient) Post() *rest.Request {
	if r.PostFunc != nil {
		return r.PostFunc()
	}
	return nil
}
func (r *mockRESTClient) Put() *rest.Request {
	if r.PutFunc != nil {
		return r.PutFunc()
	}
	return nil
}
func (r *mockRESTClient) Patch(pt types.PatchType) *rest.Request {
	if r.PatchFunc != nil {
		return r.PatchFunc(pt)
	}
	return nil
}

func (r *mockRESTClient) Get() *rest.Request {
	if r.GetFunc != nil {
		return r.GetFunc()
	}
	return nil
}

func (r *mockRESTClient) Delete() *rest.Request {
	if r.DeleteFunc != nil {
		return r.DeleteFunc()
	}
	return nil
}
func (r *mockRESTClient) APIVersion() schema.GroupVersion {
	if r.APIVersionFunc != nil {
		return r.APIVersionFunc()
	}
	return schema.GroupVersion{}
}

// mockWatch implements watch.Interface for testing
type mockWatch struct {
	ch      chan watch.Event
	stopCh  chan struct{}
	stopped bool
	stopMux sync.Mutex
}

func newMockWatch() *mockWatch {
	return &mockWatch{
		ch:     make(chan watch.Event, 10),
		stopCh: make(chan struct{}),
	}
}

func (m *mockWatch) Stop() {
	m.stopMux.Lock()
	defer m.stopMux.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
		close(m.ch)
	}
}

func (m *mockWatch) ResultChan() <-chan watch.Event {
	return m.ch
}

func (m *mockWatch) sendEvent(evt watch.Event) {
	m.stopMux.Lock()
	defer m.stopMux.Unlock()
	if !m.stopped {
		m.ch <- evt
	}
}

func TestMetricsWatchWrapper(t *testing.T) {
	tests := []struct {
		name              string
		events            []watch.Event
		expectEventCounts map[string]int
		expectErrorCounts map[string]int
	}{
		{
			name: "records ADDED event",
			events: []watch.Event{
				{Type: watch.Added, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"ADDED": 1,
			},
			expectErrorCounts: map[string]int{},
		},
		{
			name: "records MODIFIED event",
			events: []watch.Event{
				{Type: watch.Modified, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"MODIFIED": 1,
			},
			expectErrorCounts: map[string]int{},
		},
		{
			name: "records DELETED event",
			events: []watch.Event{
				{Type: watch.Deleted, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"DELETED": 1,
			},
			expectErrorCounts: map[string]int{},
		},
		{
			name: "records BOOKMARK event",
			events: []watch.Event{
				{Type: watch.Bookmark, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"BOOKMARK": 1,
			},
			expectErrorCounts: map[string]int{},
		},
		{
			name: "records ERROR event",
			events: []watch.Event{
				{Type: watch.Error, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"ERROR": 1,
			},
			expectErrorCounts: map[string]int{},
		},
		{
			name: "records nil object error",
			events: []watch.Event{
				{Type: watch.Added, Object: nil},
			},
			expectEventCounts: map[string]int{
				"ADDED": 1,
			},
			expectErrorCounts: map[string]int{
				"nil_object": 1,
			},
		},
		{
			name: "records multiple events",
			events: []watch.Event{
				{Type: watch.Added, Object: getTestObject()},
				{Type: watch.Modified, Object: getTestObject()},
				{Type: watch.Deleted, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"ADDED":    1,
				"MODIFIED": 1,
				"DELETED":  1,
			},
			expectErrorCounts: map[string]int{},
		},
		{
			name: "records mixed events and errors",
			events: []watch.Event{
				{Type: watch.Added, Object: getTestObject()},
				{Type: watch.Modified, Object: nil},
				{Type: watch.Deleted, Object: getTestObject()},
			},
			expectEventCounts: map[string]int{
				"ADDED":    1,
				"MODIFIED": 1,
				"DELETED":  1,
			},
			expectErrorCounts: map[string]int{
				"nil_object": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock metrics
			watchEventsTotal := prometheus.NewCounterVec(
				prometheus.CounterOpts{Name: "test_watch_events_total"},
				[]string{"event_type", "kind"},
			)
			watchErrorsTotal := prometheus.NewCounterVec(
				prometheus.CounterOpts{Name: "test_watch_errors_total"},
				[]string{"error_type", "kind"},
			)

			// Create mock watch
			mockW := newMockWatch()

			// Create wrapper
			ctx := context.Background()
			wrapper := &metricsWatchWrapper{
				underlying:       mockW,
				plural:           "tests",
				watchEventsTotal: watchEventsTotal,
				watchErrorsTotal: watchErrorsTotal,
				ctx:              ctx,
				ch:               make(chan watch.Event, 10),
			}

			// Start reading from ResultChan
			resultCh := wrapper.ResultChan()

			// Send events
			go func() {
				for _, evt := range tt.events {
					mockW.sendEvent(evt)
				}
			}()

			// Collect events
			var receivedEvents []watch.Event
			done := make(chan struct{})
			go func() {
				for i := 0; i < len(tt.events); i++ {
					evt := <-resultCh
					receivedEvents = append(receivedEvents, evt)
				}
				close(done)
			}()

			// Wait for events with timeout
			select {
			case <-done:
			case <-time.After(1 * time.Second):
				t.Fatal("timeout waiting for events")
			}

			// Stop the wrapper
			wrapper.Stop()

			// Verify event counts
			assert.Equal(t, len(tt.events), len(receivedEvents), "should receive all events")

			// Check metrics
			for eventType, expectedCount := range tt.expectEventCounts {
				metric, err := watchEventsTotal.GetMetricWithLabelValues(eventType, "tests")
				require.NoError(t, err)
				var m prometheus.Metric = metric
				var pb dto.Metric
				err = m.Write(&pb)
				require.NoError(t, err)
				actualCount := int(pb.GetCounter().GetValue())
				assert.Equal(t, expectedCount, actualCount, "event count mismatch for type %s", eventType)
			}

			for errorType, expectedCount := range tt.expectErrorCounts {
				metric, err := watchErrorsTotal.GetMetricWithLabelValues(errorType, "tests")
				require.NoError(t, err)
				var m prometheus.Metric = metric
				var pb dto.Metric
				err = m.Write(&pb)
				require.NoError(t, err)
				actualCount := int(pb.GetCounter().GetValue())
				assert.Equal(t, expectedCount, actualCount, "error count mismatch for type %s", errorType)
			}
		})
	}
}

func TestMetricsWatchWrapper_StopPropagation(t *testing.T) {
	// Create mock watch
	mockW := newMockWatch()

	// Create wrapper
	ctx := context.Background()
	watchEventsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_watch_events_total"},
		[]string{"event_type", "kind"},
	)

	wrapper := &metricsWatchWrapper{
		underlying:       mockW,
		plural:           "tests",
		watchEventsTotal: watchEventsTotal,
		ctx:              ctx,
		ch:               make(chan watch.Event, 10),
	}

	// Call Stop on wrapper
	wrapper.Stop()

	// Verify underlying watch was stopped
	assert.True(t, mockW.stopped, "underlying watch should be stopped")
}

func TestMetricsWatchWrapper_NoMetrics(t *testing.T) {
	// Create mock watch
	mockW := newMockWatch()

	// Create wrapper without metrics
	ctx := context.Background()
	wrapper := &metricsWatchWrapper{
		underlying:       mockW,
		plural:           "tests",
		watchEventsTotal: nil,
		watchErrorsTotal: nil,
		ctx:              ctx,
		ch:               make(chan watch.Event, 10),
	}

	// Start reading from ResultChan
	resultCh := wrapper.ResultChan()

	// Send an event
	go func() {
		mockW.sendEvent(watch.Event{Type: watch.Added, Object: getTestObject()})
	}()

	// Should still receive event
	select {
	case evt := <-resultCh:
		assert.Equal(t, watch.Added, evt.Type)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	wrapper.Stop()
}

func TestWatchResponse_KubernetesWatch_WithMetrics(t *testing.T) {
	// Create mock watch
	mockW := newMockWatch()

	// Create mock metrics
	watchEventsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_watch_events_total"},
		[]string{"event_type", "kind"},
	)
	watchErrorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_watch_errors_total"},
		[]string{"error_type", "kind"},
	)

	// Create WatchResponse with metrics
	ctx := context.Background()
	wr := &WatchResponse{
		watch:            mockW,
		ch:               make(chan resource.WatchEvent, 1),
		stopCh:           make(chan struct{}),
		ctx:              ctx,
		plural:           "tests",
		watchEventsTotal: watchEventsTotal,
		watchErrorsTotal: watchErrorsTotal,
	}

	// Call KubernetesWatch()
	k8sWatch := wr.KubernetesWatch()

	// Should return a metricsWatchWrapper
	wrapper, ok := k8sWatch.(*metricsWatchWrapper)
	require.True(t, ok, "KubernetesWatch() should return metricsWatchWrapper when metrics configured")
	assert.NotNil(t, wrapper.watchEventsTotal)
	assert.NotNil(t, wrapper.watchErrorsTotal)
	assert.Equal(t, "tests", wrapper.plural)

	// Test that metrics are recorded
	resultCh := k8sWatch.ResultChan()

	// Send an event
	go func() {
		mockW.sendEvent(watch.Event{Type: watch.Added, Object: getTestObject()})
	}()

	// Receive event
	select {
	case evt := <-resultCh:
		assert.Equal(t, watch.Added, evt.Type)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Stop the watch
	k8sWatch.Stop()

	// Verify metrics were recorded
	metric, err := watchEventsTotal.GetMetricWithLabelValues("ADDED", "tests")
	require.NoError(t, err)
	var m prometheus.Metric = metric
	var pb dto.Metric
	err = m.Write(&pb)
	require.NoError(t, err)
	actualCount := int(pb.GetCounter().GetValue())
	assert.Equal(t, 1, actualCount, "should record ADDED event metric")
}

func TestWatchResponse_KubernetesWatch_WithoutMetrics(t *testing.T) {
	// Create mock watch
	mockW := newMockWatch()

	// Create WatchResponse without metrics
	ctx := context.Background()
	wr := &WatchResponse{
		watch:            mockW,
		ch:               make(chan resource.WatchEvent, 1),
		stopCh:           make(chan struct{}),
		ctx:              ctx,
		plural:           "tests",
		watchEventsTotal: nil,
		watchErrorsTotal: nil,
	}

	// Call KubernetesWatch()
	k8sWatch := wr.KubernetesWatch()

	// Should return the raw underlying watch (not wrapped)
	assert.Equal(t, mockW, k8sWatch, "KubernetesWatch() should return raw watch when metrics not configured")
}
