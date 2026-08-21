package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

func TestConvertObjectsReturnsUnchangedObjectsWithWarning(t *testing.T) {
	raw := []byte(`{"apiVersion":"example.grafana.app/v1alpha1","kind":"Example"}`)
	req := pluginv3.ConvertObjectsRequest_builder{
		Api: pluginv3.GroupVersion_builder{
			Group:   new("example.grafana.app"),
			Version: new("v1alpha1"),
		}.Build(),
		Uid:           new("request-id"),
		TargetVersion: new("v1beta1"),
		Objects: []*pluginv3.ConvertObjectsRequest_Object{
			pluginv3.ConvertObjectsRequest_Object_builder{
				Raw: raw,
			}.Build(),
		},
	}.Build()

	response, err := (&App{}).ConvertObjects(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, "request-id", response.GetUid())
	require.Len(t, response.GetConverted(), 1)
	require.Equal(t, raw, response.GetConverted()[0].GetRaw())
	require.Equal(t, []string{"noop for now"}, response.GetConverted()[0].GetWarnings())
}
