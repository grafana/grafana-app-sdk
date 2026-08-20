package grpcplugin

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

func TestNewClientV3(t *testing.T) {
	protocol := &testClientProtocol{plugins: map[string]any{
		pluginKeyAdmission:  pluginv3.NewAdmissionServiceClient(nil),
		pluginKeyConversion: pluginv3.NewConversionServiceClient(nil),
		pluginKeyRouter:     pluginv3.NewRouteServiceClient(nil),
	}}

	client, err := NewClientV3(protocol)

	require.NoError(t, err)
	require.NotNil(t, client.AdmissionServiceClient)
	require.NotNil(t, client.ConversionServiceClient)
	require.NotNil(t, client.RouteServiceClient)
}

func TestNewClientV3ReturnsDispenseError(t *testing.T) {
	dispenseErr := errors.New("not available")
	protocol := &testClientProtocol{dispenseErr: dispenseErr}

	client, err := NewClientV3(protocol)

	require.Nil(t, client)
	require.ErrorIs(t, err, dispenseErr)
	require.ErrorContains(t, err, pluginKeyAdmission)
}

func TestNewClientV3RejectsUnexpectedClientType(t *testing.T) {
	protocol := &testClientProtocol{plugins: map[string]any{
		pluginKeyAdmission: "not an admission client",
	}}

	client, err := NewClientV3(protocol)

	require.Nil(t, client)
	require.ErrorContains(t, err, pluginKeyAdmission)
	require.ErrorContains(t, err, "unexpected client type string")
}

type testClientProtocol struct {
	plugins     map[string]any
	dispenseErr error
}

func (*testClientProtocol) Close() error { return nil }

func (c *testClientProtocol) Dispense(key string) (any, error) {
	if c.dispenseErr != nil {
		return nil, c.dispenseErr
	}
	return c.plugins[key], nil
}

func (*testClientProtocol) Ping() error { return nil }
