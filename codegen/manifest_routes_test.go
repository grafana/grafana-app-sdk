package codegen

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomRoute_ID(t *testing.T) {
	assert.Equal(t, "addStar", CustomRoute{OperationID: "addStar"}.ID())
	assert.Equal(t, "createStar", CustomRoute{Name: "createStar"}.ID())
	assert.Equal(t, "addStar", CustomRoute{Name: "createStar", OperationID: "addStar"}.ID(), "the operationId wins")
	assert.Empty(t, CustomRoute{}.ID())
}

func TestSimpleManifest_ResolveRouteIDs(t *testing.T) {
	routes := func(path, method string, route CustomRoute) map[string]map[string]CustomRoute {
		return map[string]map[string]CustomRoute{path: {method: route}}
	}
	manifest := func(versions map[string]*SimpleVersion) *SimpleManifest {
		return &SimpleManifest{AllVersions: versions}
	}

	t.Run("each route is named for its identifier", func(t *testing.T) {
		kind := VersionedKind{Kind: "Thing", Routes: map[string]map[string]CustomRoute{
			"/a": {"GET": {OperationID: "fetchA"}},
			"/b": {"GET": {Name: "getB"}, "PUT": {Name: "createB", OperationID: "addB"}},
		}}
		m := manifest(map[string]*SimpleVersion{"v1": {AllKinds: []VersionedKind{kind}}})
		require.NoError(t, m.ResolveRouteIDs())
		assert.Equal(t, "fetchA", kind.Routes["/a"]["GET"].Name)
		assert.Equal(t, "getB", kind.Routes["/b"]["GET"].Name)
		assert.Equal(t, "addB", kind.Routes["/b"]["PUT"].Name)
	})

	t.Run("a route without an identifier is an error that says where", func(t *testing.T) {
		m := manifest(map[string]*SimpleVersion{"v1": {CustomRoutes: &VersionCustomRoutes{
			Namespaced: routes("/star/{id}", "PUT", CustomRoute{}),
		}}})
		err := m.ResolveRouteIDs()
		require.ErrorContains(t, err, "custom route PUT /star/{id} of the version (namespaced) in version v1 must declare an operationId")
	})

	t.Run("an identifier is unique across the kinds and the version", func(t *testing.T) {
		m := manifest(map[string]*SimpleVersion{"v1": {
			AllKinds: []VersionedKind{{Kind: "Thing", Routes: routes("/star", "PUT", CustomRoute{OperationID: "addStar"})}},
			CustomRoutes: &VersionCustomRoutes{
				Cluster: routes("/like", "PUT", CustomRoute{Name: "addStar"}),
			},
		}})
		err := m.ResolveRouteIDs()
		require.ErrorContains(t, err, `operationId "addStar" is declared by both PUT /star of kind Thing in version v1 and PUT /like of the version (cluster) in version v1`)
	})

	t.Run("a version is its own OpenAPI document", func(t *testing.T) {
		same := func() *SimpleVersion {
			return &SimpleVersion{CustomRoutes: &VersionCustomRoutes{Namespaced: routes("/star", "PUT", CustomRoute{OperationID: "addStar"})}}
		}
		require.NoError(t, manifest(map[string]*SimpleVersion{"v1": same(), "v2": same()}).ResolveRouteIDs())
	})

	t.Run("the methods of one path are separate routes", func(t *testing.T) {
		m := manifest(map[string]*SimpleVersion{"v1": {CustomRoutes: &VersionCustomRoutes{Namespaced: map[string]map[string]CustomRoute{
			"/star": {"PUT": {OperationID: "addStar"}, "DELETE": {OperationID: "addStar"}},
		}}}})
		require.ErrorContains(t, m.ResolveRouteIDs(), "must be unique in a version")
	})
}

func TestSimpleManifest_ValidateRouteResponseKinds(t *testing.T) {
	ctx := cuecontext.New()
	manifest := func(route CustomRoute) *SimpleManifest {
		return &SimpleManifest{AllVersions: map[string]*SimpleVersion{"v1": {
			AllKinds: []VersionedKind{{Kind: "Thing"}},
			CustomRoutes: &VersionCustomRoutes{
				Namespaced: map[string]map[string]CustomRoute{"/latest": {"GET": route}},
			},
		}}}
	}

	t.Run("a route returns a kind of its version", func(t *testing.T) {
		require.NoError(t, manifest(CustomRoute{ResponseKind: "Thing", Response: ctx.CompileString("_")}).ValidateRouteResponseKinds())
	})
	t.Run("a route returns a kind that is not in its version", func(t *testing.T) {
		err := manifest(CustomRoute{ResponseKind: "Other"}).ValidateRouteResponseKinds()
		require.ErrorContains(t, err, `custom route GET /latest of the version (namespaced) in version v1 has responseKind "Other", which is not a kind of the version`)
	})
	t.Run("a route does not declare a response and a kind", func(t *testing.T) {
		err := manifest(CustomRoute{ResponseKind: "Thing", Response: ctx.CompileString("{name: string}")}).ValidateRouteResponseKinds()
		require.ErrorContains(t, err, "declares a response and a responseKind")
	})
	t.Run("a route without a kind is not checked", func(t *testing.T) {
		require.NoError(t, manifest(CustomRoute{Response: ctx.CompileString("{name: string}")}).ValidateRouteResponseKinds())
	})
}
