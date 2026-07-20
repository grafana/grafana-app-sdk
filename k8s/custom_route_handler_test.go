package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

// stubApp implements CustomRouteCaller for testing.
type stubApp struct {
	called   *app.CustomRouteRequest
	bodyRead string
	respond  func(w app.CustomRouteResponseWriter) error
	err      error
}

func (s *stubApp) CallCustomRoute(_ context.Context, w app.CustomRouteResponseWriter, r *app.CustomRouteRequest) error {
	s.called = r
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		s.bodyRead = string(b)
	}
	if s.respond != nil {
		return s.respond(w)
	}
	return s.err
}

func testManifest() app.ManifestData {
	return app.ManifestData{
		Group: "test.grafana.app",
		Versions: []app.ManifestVersion{{
			Name: "v1",
			Kinds: []app.ManifestVersionKind{{
				Kind:   "Foo",
				Plural: "foos",
				Scope:  string(resource.NamespacedScope),
			}, {
				Kind:   "Cluz",
				Plural: "cluzes",
				Scope:  string(resource.ClusterScope),
			}},
		}},
	}
}

func TestNewCustomRouteHandler(t *testing.T) {
	t.Run("missing app", func(t *testing.T) {
		_, err := NewCustomRouteHandler(CustomRouteHandlerConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Caller")
	})
	t.Run("duplicate plural", func(t *testing.T) {
		_, err := NewCustomRouteHandler(CustomRouteHandlerConfig{
			Caller: &stubApp{},
			Manifest: app.ManifestData{
				Group: "g",
				Versions: []app.ManifestVersion{{Name: "v1", Kinds: []app.ManifestVersionKind{
					{Kind: "A", Plural: "things", Scope: "Namespaced"},
					{Kind: "B", Plural: "things", Scope: "Namespaced"},
				}}},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate plural")
	})
	t.Run("ok", func(t *testing.T) {
		h, err := NewCustomRouteHandler(CustomRouteHandlerConfig{
			Caller:   &stubApp{},
			Manifest: testManifest(),
		})
		require.NoError(t, err)
		require.NotNil(t, h)
	})
}

// newTestMux returns a parent ServeMux with the handler's patterns already installed via
// Register. Tests dispatch through the parent mux, exercising the public composition path.
func newTestMux(t *testing.T, stub *stubApp) *http.ServeMux {
	t.Helper()
	h, err := NewCustomRouteHandler(CustomRouteHandlerConfig{
		Caller:   stub,
		Manifest: testManifest(),
	})
	require.NoError(t, err)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestCustomRouteHandler_Dispatch(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		wantGroup     string
		wantVersion   string
		wantNamespace string
		wantKind      string
		wantPlural    string
		wantName      string
		wantSubPath   string
	}{
		{
			name:      "cluster-scoped single-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/things",
			wantGroup: "test.grafana.app", wantVersion: "v1",
			wantSubPath: "things",
		},
		{
			name:      "cluster-scoped multi-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/things/nested",
			wantGroup: "test.grafana.app", wantVersion: "v1",
			wantSubPath: "things/nested",
		},
		{
			name:      "cluster-scoped subresource single-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/cluzes/mycluz/details",
			wantGroup: "test.grafana.app", wantVersion: "v1",
			wantKind: "Cluz", wantPlural: "cluzes", wantName: "mycluz", wantSubPath: "details",
		},
		{
			name:      "cluster-scoped subresource multi-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/cluzes/mycluz/blah/details",
			wantGroup: "test.grafana.app", wantVersion: "v1",
			wantKind: "Cluz", wantPlural: "cluzes", wantName: "mycluz", wantSubPath: "blah/details",
		},
		{
			name:      "namespace-scoped single-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/namespaces/ns1/things",
			wantGroup: "test.grafana.app", wantVersion: "v1", wantNamespace: "ns1",
			wantSubPath: "things",
		},
		{
			name:      "namespace-scoped multi-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/namespaces/ns1/things/nested",
			wantGroup: "test.grafana.app", wantVersion: "v1", wantNamespace: "ns1",
			wantSubPath: "things/nested",
		},
		{
			name:      "namespace-scoped subresource single-segment route",
			method:    http.MethodPost,
			path:      "/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/reboot",
			wantGroup: "test.grafana.app", wantVersion: "v1", wantNamespace: "ns1",
			wantKind: "Foo", wantPlural: "foos", wantName: "myfoo", wantSubPath: "reboot",
		},
		{
			name:      "namespace-scoped subresource multi-segment route",
			method:    http.MethodGet,
			path:      "/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/logs/stream",
			wantGroup: "test.grafana.app", wantVersion: "v1", wantNamespace: "ns1",
			wantKind: "Foo", wantPlural: "foos", wantName: "myfoo", wantSubPath: "logs/stream",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubApp{}
			mux := newTestMux(t, stub)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			require.NotNil(t, stub.called, "CallCustomRoute was not invoked")
			id := stub.called.ResourceIdentifier
			assert.Equal(t, tt.wantGroup, id.Group)
			assert.Equal(t, tt.wantVersion, id.Version)
			assert.Equal(t, tt.wantNamespace, id.Namespace)
			assert.Equal(t, tt.wantKind, id.Kind)
			assert.Equal(t, tt.wantPlural, id.Plural)
			assert.Equal(t, tt.wantName, id.Name)
			assert.Equal(t, tt.wantSubPath, stub.called.Path)
			assert.Equal(t, tt.method, stub.called.Method)
		})
	}
}

func TestCustomRouteHandler_BodyHeadersURL(t *testing.T) {
	stub := &stubApp{}
	mux := newTestMux(t, stub)
	req := httptest.NewRequest(http.MethodPost,
		"/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/echo?q=1",
		strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("X-Test", "yes")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.NotNil(t, stub.called)
	assert.Equal(t, `{"hello":"world"}`, stub.bodyRead)
	assert.Equal(t, "yes", stub.called.Headers.Get("X-Test"))
	require.NotNil(t, stub.called.URL)
	assert.Equal(t, "1", stub.called.URL.Query().Get("q"))
}

func TestCustomRouteHandler_Errors(t *testing.T) {
	t.Run("malformed /apis path", func(t *testing.T) {
		// The /apis/ catch-all installed by Register returns a metav1.Status JSON 404 for any
		// /apis/... URL that doesn't match a more-specific pattern.
		mux := newTestMux(t, &stubApp{})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/apis/", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		var st metav1.Status
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &st))
		assert.Equal(t, metav1.StatusFailure, st.Status)
	})
	t.Run("plural with no name dispatches as version-level", func(t *testing.T) {
		// A URL like /apis/.../namespaces/ns1/foos with no /<name>/<sub> tail no longer
		// matches the literal-plural subresource pattern; it falls through to the version-level
		// fallback with Path="foos". The caller is expected to return ErrCustomRouteNotFound,
		// which the handler translates to a JSON 404.
		stub := &stubApp{err: app.ErrCustomRouteNotFound}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v1/namespaces/ns1/foos", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		require.NotNil(t, stub.called)
		assert.Equal(t, "foos", stub.called.Path)
		assert.Empty(t, stub.called.ResourceIdentifier.Kind)
	})
	t.Run("namespaces/ with no namespace dispatches as version-level", func(t *testing.T) {
		// /apis/g/v1/namespaces/ doesn't match the namespaced pattern (no value for {namespace}).
		// It falls through to the cluster version-level fallback with Path="namespaces/", and
		// the caller's not-found result becomes a JSON 404.
		stub := &stubApp{err: app.ErrCustomRouteNotFound}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v1/namespaces/", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
	t.Run("unknown group/version short-circuits to 404", func(t *testing.T) {
		// Patterns are registered with literal group/version, so a URL for a group or version
		// not in the manifest doesn't match any version-specific pattern. The /apis/ catch-all
		// returns a JSON 404 without invoking the caller.
		stub := &stubApp{}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/other.grafana.app/v1/anything", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Nil(t, stub.called, "caller should not be invoked for unknown group")

		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v9/anything", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Nil(t, stub.called, "caller should not be invoked for unknown version")
	})
	t.Run("ErrCustomRouteNotFound -> 404", func(t *testing.T) {
		stub := &stubApp{err: app.ErrCustomRouteNotFound}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/missing", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		var st metav1.Status
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &st))
		assert.Equal(t, metav1.StatusReasonNotFound, st.Reason)
	})
	t.Run("generic error -> 500", func(t *testing.T) {
		stub := &stubApp{err: errors.New("boom")}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/sub", nil))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
	t.Run("APIStatus error preserves code", func(t *testing.T) {
		stub := &stubApp{err: apierrors.NewBadRequest("nope")}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/sub", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("handler writes response without error", func(t *testing.T) {
		stub := &stubApp{respond: func(w app.CustomRouteResponseWriter) error {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("brewing"))
			return nil
		}}
		mux := newTestMux(t, stub)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
			"/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/sub", nil))
		assert.Equal(t, http.StatusTeapot, rec.Code)
		assert.Equal(t, "brewing", rec.Body.String())
	})
}

func TestCustomRouteHandler_Register(t *testing.T) {
	stub := &stubApp{}
	h, err := NewCustomRouteHandler(CustomRouteHandlerConfig{Caller: stub, Manifest: testManifest()})
	require.NoError(t, err)
	mux := http.NewServeMux()
	h.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/apis/test.grafana.app/v1/namespaces/ns1/foos/myfoo/echo")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NotNil(t, stub.called)
	assert.Equal(t, "echo", stub.called.Path)
}
