package simple

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestRouteTemplateMatch(t *testing.T) {
	tests := []struct {
		template string
		path     string
		match    bool
	}{
		{"flags/{key}", "flags/dark-mode", true},
		{"flags/{key}", "/flags/dark-mode", true},
		{"flags/{key}", "flags", false},
		{"flags/{key}", "flags/", false},
		{"flags/{key}", "flags/a/b", false},
		{"flags/{key}/eval", "flags/a/eval", true},
		{"flags/{key}/eval", "flags/a/other", false},
		{"files/{path:*}", "files/a/b/c.txt", true},
		{"files/{path...}", "files/a/b/c.txt", true},
		{"files/{path:*}", "files/a", true},
		{"files/{path:*}", "files/", true},
		{"files/{path:*}", "files", true},
		{"files/{path:*}", "other/a", false},
		{"things/{name}/files/{path:*}", "things/x/files/deep/er", true},
	}
	for _, tc := range tests {
		tmpl, ok := newRouteTemplate(tc.template, nil)
		require.True(t, ok, tc.template)
		assert.Equal(t, tc.match, tmpl.match(tc.path), "%s against %s", tc.template, tc.path)
	}

	_, ok := newRouteTemplate("files/list", nil)
	assert.False(t, ok, "a path without parameters is looked up exactly")
}

// The host sends the concrete path a route was called with, so a route
// declared with parameters must still find its handler.
func TestApp_CallCustomRouteMatchesParameters(t *testing.T) {
	kind := testKind()
	called := func(name string) AppCustomRouteHandler {
		return func(_ context.Context, w app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
			_, err := w.Write([]byte(name))
			return err
		}
	}
	a := createTestApp(t, AppConfig{
		ManagedKinds: []AppManagedKind{{
			Kind: kind,
			CustomRoutes: AppCustomRouteHandlers{
				{Method: AppCustomRouteMethodGet, Path: "files/{path:*}"}: called("kind-files"),
				{Method: AppCustomRouteMethodGet, Path: "files/index"}:    called("kind-index"),
			},
		}},
		VersionedCustomRoutes: map[string]AppVersionRouteHandlers{
			kind.Version(): {
				{Namespaced: true, Method: AppCustomRouteMethodGet, Path: "flags/{key}"}:         called("flag"),
				{Namespaced: true, Method: AppCustomRouteMethodGet, Path: "flags/{key}/history"}: called("flag-history"),
				{Namespaced: true, Method: AppCustomRouteMethodGet, Path: "flags/{key}/{field}"}: called("flag-field"),
				{Namespaced: true, Method: AppCustomRouteMethodGet, Path: "blobs/{path...}"}:     called("blobs"),
				{Namespaced: false, Method: AppCustomRouteMethodGet, Path: "flags/{key}"}:        called("cluster-flag"),
			},
		},
	})

	call := func(id resource.FullIdentifier, method, path string) (string, error) {
		rw := httptest.NewRecorder()
		err := a.CallCustomRoute(context.TODO(), rw, &app.CustomRouteRequest{
			ResourceIdentifier: id, Method: method, Path: path,
		})
		return rw.Body.String(), err
	}
	version := resource.FullIdentifier{Group: kind.Group(), Version: kind.Version(), Namespace: "ns"}
	cluster := resource.FullIdentifier{Group: kind.Group(), Version: kind.Version()}
	object := resource.FullIdentifier{Group: kind.Group(), Version: kind.Version(), Kind: kind.Kind(), Namespace: "ns", Name: "obj"}

	for _, tc := range []struct {
		name string
		id   resource.FullIdentifier
		path string
		want string
	}{
		{"one parameter", version, "flags/dark-mode", "flag"},
		{"a literal segment wins over a parameter", version, "flags/dark-mode/history", "flag-history"},
		{"two parameters", version, "flags/dark-mode/owner", "flag-field"},
		{"catch-all alias", version, "blobs/a/b/c", "blobs"},
		{"cluster scope is separate", cluster, "flags/dark-mode", "cluster-flag"},
		{"kind catch-all", object, "files/2026/q3.csv", "kind-files"},
		{"an exact path wins over a catch-all", object, "files/index", "kind-index"},
		{"the declared path still matches", version, "flags/{key}", "flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := call(tc.id, http.MethodGet, tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("no match", func(t *testing.T) {
		_, err := call(version, http.MethodGet, "other/thing")
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
		_, err = call(version, http.MethodPost, "flags/dark-mode")
		assert.Equal(t, app.ErrCustomRouteNotFound, err, "the method is part of the match")
	})
}
