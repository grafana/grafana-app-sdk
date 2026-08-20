package grpcplugin

import (
	"github.com/hashicorp/go-plugin"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

type ClientV3 struct {
	pluginv3.AdmissionServiceClient
	pluginv3.ConversionServiceClient
	pluginv3.RouteServiceClient
}

func NewClientV3(rpcClient plugin.ClientProtocol) (*ClientV3, error) {
	c := &ClientV3{}
	// Admission
	raw, err := rpcClient.Dispense(pluginKeyAdmission)
	if err != nil {
		return nil, err
	}
	if raw != nil {
		if client, ok := raw.(pluginv3.AdmissionServiceClient); ok {
			c.AdmissionServiceClient = client
		}
	}

	// Conversion
	raw, err = rpcClient.Dispense(pluginKeyConversion)
	if err != nil {
		return nil, err
	}
	if raw != nil {
		if client, ok := raw.(pluginv3.ConversionServiceClient); ok {
			c.ConversionServiceClient = client
		}
	}

	// Router
	raw, err = rpcClient.Dispense(pluginKeyRouter)
	if err != nil {
		return nil, err
	}
	if raw != nil {
		if client, ok := raw.(pluginv3.RouteServiceClient); ok {
			c.RouteServiceClient = client
		}
	}

	return c, nil
}
