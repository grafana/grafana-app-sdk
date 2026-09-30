package grpcplugin

import (
	"errors"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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

func TestNewClientV3FromConn(t *testing.T) {
	_, err := NewClientV3FromConn(nil, ClientV3Options{})
	require.ErrorContains(t, err, "a gRPC connection is required")

	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client, err := NewClientV3FromConn(conn, ClientV3Options{})
	require.NoError(t, err)
	_, ok := client.(*clientV3)
	require.True(t, ok, "without a token exchanger, requests are not authenticated")

	client, err = NewClientV3FromConn(conn, ClientV3Options{TokenExchanger: authn.NewStaticTokenExchanger("token"), PluginID: "example-app"})
	require.NoError(t, err)
	_, ok = client.(*authenticatedClientV3)
	require.True(t, ok)

	_, err = NewClientV3FromConn(conn, ClientV3Options{TokenExchanger: authn.NewStaticTokenExchanger("token")})
	require.ErrorContains(t, err, "the plugin ID or the plugin's API groups are required")
}
