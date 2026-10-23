package k8s

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/logging"
)

func TestStreamConnectionError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		expectErr string
	}{
		{
			name:      "wraps error",
			err:       errors.New("stream error: test"),
			expectErr: "stream error: test",
		},
		{
			name:      "unwraps correctly",
			err:       errors.New("original error"),
			expectErr: "original error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamErr := &streamConnectionError{err: tt.err}

			// Test Error() method
			assert.Equal(t, tt.expectErr, streamErr.Error())

			// Test Unwrap() method
			assert.Equal(t, tt.err, streamErr.Unwrap())

			// Test Timeout() method - should always return true
			assert.True(t, streamErr.Timeout())

			// Test Temporary() method - should always return true
			assert.True(t, streamErr.Temporary())
		})
	}
}

func TestIsStreamError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "stream error",
			err:      errors.New("stream error: something went wrong"),
			expected: true,
		},
		{
			name:     "INTERNAL_ERROR",
			err:      errors.New("http2: server sent INTERNAL_ERROR"),
			expected: true,
		},
		{
			name:     "received from peer",
			err:      errors.New("error received from peer"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("write: broken pipe"),
			expected: true,
		},
		{
			name:     "GOAWAY frame",
			err:      errors.New("http2: server sent GOAWAY and closed the connection"),
			expected: true,
		},
		{
			name:     "non-stream error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStreamError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockRoundTripper is a mock HTTP RoundTripper for testing
type mockRoundTripper struct {
	resp *http.Response
	err  error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.resp, m.err
}

func TestStreamErrorTransport(t *testing.T) {
	ctx := logging.Context(context.Background(), &logging.NoOpLogger{})

	tests := []struct {
		name             string
		baseErr          error
		baseResp         *http.Response
		expectErr        error
		expectStreamErr  bool
		expectBodyWrap   bool
	}{
		{
			name:             "no error returns response with wrapped body",
			baseErr:          nil,
			baseResp:         &http.Response{Body: io.NopCloser(strings.NewReader("test"))},
			expectErr:        nil,
			expectStreamErr:  false,
			expectBodyWrap:   true,
		},
		{
			name:             "stream error gets wrapped",
			baseErr:          errors.New("stream error: test failure"),
			baseResp:         nil,
			expectErr:        nil, // Will be a streamConnectionError
			expectStreamErr:  true,
			expectBodyWrap:   false,
		},
		{
			name:             "non-stream error passes through",
			baseErr:          errors.New("some other error"),
			baseResp:         nil,
			expectErr:        errors.New("some other error"),
			expectStreamErr:  false,
			expectBodyWrap:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseTransport := &mockRoundTripper{
				resp: tt.baseResp,
				err:  tt.baseErr,
			}

			transport := &streamErrorTransport{
				base: baseTransport,
			}

			req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
			require.NoError(t, err)

			resp, err := transport.RoundTrip(req)

			if tt.expectStreamErr {
				require.Error(t, err)
				var streamErr *streamConnectionError
				require.True(t, errors.As(err, &streamErr))
				assert.True(t, streamErr.Timeout())
				assert.True(t, streamErr.Temporary())
			} else if tt.expectErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}

			if tt.expectBodyWrap {
				require.NotNil(t, resp)
				require.NotNil(t, resp.Body)
				_, ok := resp.Body.(*streamErrorReadCloser)
				assert.True(t, ok, "response body should be wrapped in streamErrorReadCloser")
			}
		})
	}
}

// mockReadCloser is a mock io.ReadCloser for testing
type mockReadCloser struct {
	readErr error
	closed  bool
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	return 0, m.readErr
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

func TestStreamErrorReadCloser(t *testing.T) {
	ctx := logging.Context(context.Background(), &logging.NoOpLogger{})

	tests := []struct {
		name            string
		readErr         error
		expectStreamErr bool
	}{
		{
			name:            "stream error gets wrapped",
			readErr:         errors.New("stream error: connection lost"),
			expectStreamErr: true,
		},
		{
			name:            "INTERNAL_ERROR gets wrapped",
			readErr:         errors.New("http2: INTERNAL_ERROR"),
			expectStreamErr: true,
		},
		{
			name:            "non-stream error passes through",
			readErr:         errors.New("regular error"),
			expectStreamErr: false,
		},
		{
			name:            "EOF passes through",
			readErr:         io.EOF,
			expectStreamErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseReadCloser := &mockReadCloser{
				readErr: tt.readErr,
			}

			wrapper := &streamErrorReadCloser{
				ReadCloser: baseReadCloser,
				context:    ctx,
			}

			buf := make([]byte, 10)
			_, err := wrapper.Read(buf)

			require.Error(t, err)

			if tt.expectStreamErr {
				var streamErr *streamConnectionError
				require.True(t, errors.As(err, &streamErr))
				assert.True(t, streamErr.Timeout())
				assert.True(t, streamErr.Temporary())
			} else {
				var streamErr *streamConnectionError
				assert.False(t, errors.As(err, &streamErr))
			}
		})
	}
}

func TestWrapWithStreamErrorHandling(t *testing.T) {
	tests := []struct {
		name                string
		existingWrapTransport func(http.RoundTripper) http.RoundTripper
		expectChained       bool
	}{
		{
			name:                "no existing wrap transport",
			existingWrapTransport: nil,
			expectChained:       false,
		},
		{
			name: "existing wrap transport gets chained",
			existingWrapTransport: func(rt http.RoundTripper) http.RoundTripper {
				return rt // Identity wrapper for testing
			},
			expectChained: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &rest.Config{
				WrapTransport: tt.existingWrapTransport,
			}

			WrapWithStreamErrorHandling(cfg)

			require.NotNil(t, cfg.WrapTransport)

			// Test that the wrapper works
			baseTransport := &mockRoundTripper{
				err: errors.New("stream error: test"),
			}

			ctx := logging.Context(context.Background(), &logging.NoOpLogger{})
			req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
			require.NoError(t, err)

			wrappedTransport := cfg.WrapTransport(baseTransport)
			require.NotNil(t, wrappedTransport)

			_, err = wrappedTransport.RoundTrip(req)
			require.Error(t, err)

			var streamErr *streamConnectionError
			assert.True(t, errors.As(err, &streamErr))
		})
	}
}
