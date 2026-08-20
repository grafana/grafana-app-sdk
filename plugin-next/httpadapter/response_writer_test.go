package httpadapter

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseWriterWriteAndHeaders(t *testing.T) {
	t.Run("writes a body with implicit status and detected content type", func(t *testing.T) {
		rw := newResponseWriter(newTestCallRouteResponseSender())

		n, err := rw.Write([]byte(`{"message":"hello"}`))

		require.NoError(t, err)
		require.Equal(t, len(`{"message":"hello"}`), n)
		require.Equal(t, http.StatusOK, rw.code)
		require.True(t, rw.wroteHeader)
		require.Equal(t, "text/plain; charset=utf-8", rw.Header().Get("Content-Type"))
		require.Equal(t, `{"message":"hello"}`, rw.body.String())
	})

	t.Run("preserves an explicit content type", func(t *testing.T) {
		rw := newResponseWriter(newTestCallRouteResponseSender())
		rw.Header().Set("Content-Type", "application/custom")

		_, err := rw.Write([]byte("body"))

		require.NoError(t, err)
		require.Equal(t, "application/custom", rw.Header().Get("Content-Type"))
	})

	t.Run("does not add content type when transfer encoding is set", func(t *testing.T) {
		rw := newResponseWriter(newTestCallRouteResponseSender())
		rw.Header().Set("Transfer-Encoding", "chunked")

		_, err := rw.Write([]byte("body"))

		require.NoError(t, err)
		require.Empty(t, rw.Header().Get("Content-Type"))
	})

	t.Run("only honors the first status", func(t *testing.T) {
		rw := newResponseWriter(newTestCallRouteResponseSender())

		rw.WriteHeader(http.StatusCreated)
		rw.WriteHeader(http.StatusTeapot)
		_, err := rw.Write([]byte("body"))

		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, rw.code)
	})
}

func TestResponseWriterHeaderInitializesNilMap(t *testing.T) {
	rw := newResponseWriter(newTestCallRouteResponseSender())
	rw.header = nil

	header := rw.Header()
	header.Set("X-Test", "value")

	require.Equal(t, "value", rw.header.Get("X-Test"))
}

func TestResponseWriterFlush(t *testing.T) {
	t.Run("streams headers once and each body chunk once", func(t *testing.T) {
		sender := newTestCallRouteResponseSender()
		rw := newResponseWriter(sender)
		rw.Header().Add("X-Test", "one")
		rw.Header().Add("X-Test", "two")
		rw.WriteHeader(http.StatusCreated)
		_, err := rw.Write([]byte("first"))
		require.NoError(t, err)

		rw.Flush()
		require.Empty(t, rw.body.Bytes())

		_, err = rw.Write([]byte("second"))
		require.NoError(t, err)
		rw.Flush()
		rw.Flush() // An empty flush after the first chunk sends nothing.

		require.Len(t, sender.respMessages, 2)
		first := sender.respMessages[0]
		require.Equal(t, int32(http.StatusCreated), first.GetCode())
		require.Equal(t, []string{"one", "two"}, first.GetHeaders()["X-Test"].GetValues())
		require.Equal(t, "first", string(first.GetBody()))

		second := sender.respMessages[1]
		require.Zero(t, second.GetCode())
		require.Empty(t, second.GetHeaders())
		require.Equal(t, "second", string(second.GetBody()))
	})

	t.Run("sends an empty initial response with default status", func(t *testing.T) {
		sender := newTestCallRouteResponseSender()
		rw := newResponseWriter(sender)

		rw.Flush()

		require.Len(t, sender.respMessages, 1)
		require.Equal(t, int32(http.StatusOK), sender.respMessages[0].GetCode())
		require.Empty(t, sender.respMessages[0].GetBody())
		require.NotNil(t, rw.header)
		require.Empty(t, rw.body.Bytes())
		require.True(t, rw.wroteHeader)
		require.Nil(t, rw.chunk())

		// Flush commits the implicit status and headers.
		rw.WriteHeader(http.StatusCreated)
		require.Equal(t, http.StatusOK, rw.code)
	})

	t.Run("retains a stream send error", func(t *testing.T) {
		sender := newTestCallRouteResponseSender()
		sender.sendErr = errors.New("send failed")
		rw := newResponseWriter(sender)

		require.NotPanics(t, rw.Flush)
		require.ErrorIs(t, rw.sendErr, sender.sendErr)
		require.Len(t, sender.respMessages, 1)

		n, err := rw.Write([]byte("not sent"))
		require.Zero(t, n)
		require.ErrorIs(t, err, sender.sendErr)
		rw.Flush()
		require.Len(t, sender.respMessages, 1)
	})
}
