package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const v2TestPluginID = "my-app"

// v2Recorder is the plugin's handler: it records the context a call reached it with.
type v2Recorder struct {
	ctx context.Context
}

var nopSender = backend.CallResourceResponseSenderFunc(func(*backend.CallResourceResponse) error { return nil })

func v2Resource(namespace string) *backend.CallResourceRequest {
	return &backend.CallResourceRequest{
		PluginContext: backend.PluginContext{
			PluginID:  v2TestPluginID,
			Namespace: namespace,
			AppInstanceSettings: &backend.AppInstanceSettings{
				JSONData:                []byte(`{"message":"hello"}`),
				DecryptedSecureJSONData: map[string]string{"apiKey": "s3cret"},
				Updated:                 time.UnixMilli(1700000000000),
				APIVersion:              "v0alpha1",
			},
		},
		Path: "clients",
	}
}

func TestSettingsMiddlewareRouteRequestInfo(t *testing.T) {
	rec := &v2Recorder{}
	h, err := backend.HandlerFromMiddlewares(&backend.Handlers{CallResourceHandler: backend.CallResourceHandlerFunc(func(ctx context.Context, _ *backend.CallResourceRequest, _ backend.CallResourceResponseSender) error {
		rec.ctx = ctx
		return nil
	})}, newSettingsMiddleware())
	require.NoError(t, err)

	t.Run("resource calls get the app instance settings", func(t *testing.T) {
		require.NoError(t, h.CallResource(context.Background(), v2Resource("default"), nopSender))
		info := resource.RouteRequestInfoFrom(rec.ctx)
		require.NotNil(t, info)
		assert.Equal(t, resource.FullIdentifier{
			Namespace: "default", Name: "instance", Group: v2TestPluginID, Version: "v0alpha1", Kind: "Settings", Plural: "app",
		}, info.FullIdentifier)
		assert.Equal(t, "1700000000000", info.ResourceVersion)
		assert.JSONEq(t, `{
			"apiVersion": "my-app/v0alpha1",
			"kind": "Settings",
			"metadata": {"name": "instance", "namespace": "default"},
			"spec": {"jsonData": {"message": "hello"}}
		}`, string(info.Parent))
		assert.Equal(t, resource.DecryptedSecureValues{"apiKey": "s3cret"}, info.DecryptedSecureValues)

		// The secure values never appear when the info is serialized.
		raw, err := json.Marshal(info)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "s3cret")
	})

	t.Run("the plugin context's API version wins", func(t *testing.T) {
		req := v2Resource("default")
		req.PluginContext.APIVersion = "v1"
		require.NoError(t, h.CallResource(context.Background(), req, nopSender))
		assert.Equal(t, "v1", resource.RouteRequestInfoFrom(rec.ctx).Version)
	})

	t.Run("calls without app instance settings get none", func(t *testing.T) {
		req := v2Resource("default")
		req.PluginContext.AppInstanceSettings = nil
		require.NoError(t, h.CallResource(context.Background(), req, nopSender))
		assert.Nil(t, resource.RouteRequestInfoFrom(rec.ctx))
	})

	t.Run("route info already in the context is kept", func(t *testing.T) {
		existing := &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Group: "other"}}
		require.NoError(t, h.CallResource(resource.WithRouteRequestInfo(context.Background(), existing), v2Resource("default"), nopSender))
		assert.Same(t, existing, resource.RouteRequestInfoFrom(rec.ctx))
	})
}
