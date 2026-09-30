package grpcplugin

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

func TestNewClientV3(t *testing.T) {
	protocol := &testClientProtocol{plugins: map[string]any{
		pluginKeyAdmission:  pluginv3.NewAdmissionServiceClient(nil),
		pluginKeyConversion: pluginv3.NewConversionServiceClient(nil),
		pluginKeyRouter:     pluginv3.NewRouteServiceClient(nil),
	}}

	client, err := NewClientV3(protocol, ClientV3Options{})

	require.NoError(t, err)
	impl, ok := client.(*clientV3)
	require.True(t, ok)
	require.NotNil(t, impl.admission)
	require.NotNil(t, impl.conversion)
	require.NotNil(t, impl.route)
}

func TestNewClientV3ReturnsDispenseError(t *testing.T) {
	dispenseErr := errors.New("not available")
	protocol := &testClientProtocol{dispenseErr: dispenseErr}

	client, err := NewClientV3(protocol, ClientV3Options{})

	require.Nil(t, client)
	require.ErrorIs(t, err, dispenseErr)
	require.ErrorContains(t, err, pluginKeyAdmission)
}

func TestNewClientV3RejectsUnexpectedClientType(t *testing.T) {
	protocol := &testClientProtocol{plugins: map[string]any{
		pluginKeyAdmission: "not an admission client",
	}}

	client, err := NewClientV3(protocol, ClientV3Options{})

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
