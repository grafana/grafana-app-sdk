package grpcplugin

import (
	"context"
	"net"
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/grafana/grafana-app-sdk/k8s"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

func TestCallerIDTokenIsPassedToEveryService(t *testing.T) {
	calls := map[string]func(context.Context, *ClientV3) error{
		"admission": func(ctx context.Context, c *ClientV3) error {
			_, err := c.AdmissionReview(ctx, &pluginv3.AdmissionReviewRequest{})
			return err
		},
		"conversion": func(ctx context.Context, c *ClientV3) error {
			_, err := c.ConvertObjects(ctx, &pluginv3.ConvertObjectsRequest{})
			return err
		},
		"route": func(ctx context.Context, c *ClientV3) error {
			stream, err := c.CallRoute(ctx, &pluginv3.CallRouteRequest{})
			if err != nil {
				return err
			}
			_, err = stream.Recv()
			return err
		},
	}
	tests := []struct {
		name        string
		ctx         context.Context
		wantIDToken string
	}{
		{name: "caller", ctx: k8s.WithIDToken(context.Background(), "caller-id-token"), wantIDToken: "caller-id-token"},
		{name: "no caller", ctx: context.Background()},
		// The context is the source of truth for who the caller is.
		{
			name:        "replaces metadata already set",
			ctx:         metadata.AppendToOutgoingContext(k8s.WithIDToken(context.Background(), "caller-id-token"), idTokenMetadataKey, "other-id-token"),
			wantIDToken: "caller-id-token",
		},
	}
	for service, call := range calls {
		for _, tt := range tests {
			t.Run(service+"/"+tt.name, func(t *testing.T) {
				server := &idTokenRecordingServer{}
				client := newTestClientV3(t, server)

				require.NoError(t, call(tt.ctx, client))
				assert.Equal(t, tt.wantIDToken, server.idToken)
				assert.Equal(t, tt.wantIDToken != "", server.hasIDToken)
			})
		}
	}
}

// newTestClientV3 serves server over an in-memory connection with the same
// go-plugin registrations Grafana and plugins use.
func newTestClientV3(t *testing.T, server V3Server) *ClientV3 {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	for _, p := range (ServeOpts{AdmissionServer: server, ConversionServer: server, RouteServer: server}).PluginSet() {
		require.NoError(t, p.(plugin.GRPCPlugin).GRPCServer(nil, grpcServer))
	}
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	clients := make(map[string]any)
	for key, p := range ClientPluginSet() {
		c, err := p.(plugin.GRPCPlugin).GRPCClient(t.Context(), nil, conn)
		require.NoError(t, err)
		clients[key] = c
	}
	client, err := NewClientV3(&testClientProtocol{plugins: clients})
	require.NoError(t, err)
	return client
}

type idTokenRecordingServer struct {
	UnimplementedV3Server

	idToken    string
	hasIDToken bool
}

func (s *idTokenRecordingServer) record(ctx context.Context) {
	s.idToken, s.hasIDToken = k8s.IDTokenFromContext(ctx)
}

func (s *idTokenRecordingServer) AdmissionReview(ctx context.Context, _ *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	s.record(ctx)
	return &pluginv3.AdmissionReviewResponse{}, nil
}

func (s *idTokenRecordingServer) ConvertObjects(ctx context.Context, _ *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	s.record(ctx)
	return &pluginv3.ConvertObjectsResponse{}, nil
}

func (s *idTokenRecordingServer) CallRoute(_ *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	s.record(stream.Context())
	return stream.Send(&pluginv3.CallRouteResponse{})
}
