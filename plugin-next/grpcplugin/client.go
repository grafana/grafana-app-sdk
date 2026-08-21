package grpcplugin

import (
	"fmt"

	"github.com/hashicorp/go-plugin"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// ClientV3 groups clients for the grafana.plugin.v3 services.
type ClientV3 struct {
	pluginv3.AdmissionServiceClient
	pluginv3.ConversionServiceClient
	pluginv3.RouteServiceClient
}

// NewClientV3 dispenses clients for all grafana.plugin.v3 services from a
// negotiated go-plugin client connection.
func NewClientV3(rpcClient plugin.ClientProtocol) (*ClientV3, error) {
	admission, err := dispense[pluginv3.AdmissionServiceClient](rpcClient, pluginKeyAdmission)
	if err != nil {
		return nil, err
	}

	conversion, err := dispense[pluginv3.ConversionServiceClient](rpcClient, pluginKeyConversion)
	if err != nil {
		return nil, err
	}

	router, err := dispense[pluginv3.RouteServiceClient](rpcClient, pluginKeyRouter)
	if err != nil {
		return nil, err
	}

	return &ClientV3{
		AdmissionServiceClient:  admission,
		ConversionServiceClient: conversion,
		RouteServiceClient:      router,
	}, nil
}

func dispense[T any](rpcClient plugin.ClientProtocol, key string) (T, error) {
	var zero T
	raw, err := rpcClient.Dispense(key)
	if err != nil {
		return zero, fmt.Errorf("dispense plugin %q: %w", key, err)
	}

	client, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("dispense plugin %q: unexpected client type %T", key, raw)
	}
	return client, nil
}
