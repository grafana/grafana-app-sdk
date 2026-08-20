package grpcplugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServeOptsPluginSet(t *testing.T) {
	server := UnimplementedV3Server{}

	tests := []struct {
		name string
		opts ServeOpts
		key  string
	}{
		{name: "admission", opts: ServeOpts{AdmissionServer: server}, key: pluginKeyAdmission},
		{name: "conversion", opts: ServeOpts{ConversionServer: server}, key: pluginKeyConversion},
		{name: "route", opts: ServeOpts{RouteServer: server}, key: pluginKeyRouter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugins := tt.opts.PluginSet()
			require.Len(t, plugins, 1)
			require.Contains(t, plugins, tt.key)
		})
	}
}

func TestClientPluginSet(t *testing.T) {
	plugins := ClientPluginSet()

	require.Len(t, plugins, 3)
	require.Contains(t, plugins, pluginKeyAdmission)
	require.Contains(t, plugins, pluginKeyConversion)
	require.Contains(t, plugins, pluginKeyRouter)
}
