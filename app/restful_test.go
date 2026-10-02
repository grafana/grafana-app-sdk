package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emicklei/go-restful/v3"
	"github.com/stretchr/testify/require"
)

func TestRestfulRoutesWebService(t *testing.T) {
	service := func(path string) *restful.WebService {
		ws := new(restful.WebService).Path(path).Produces(restful.MIME_JSON)
		ws.Route(ws.GET("").To(func(req *restful.Request, resp *restful.Response) {
			_ = resp.WriteEntity(map[string]string{"namespace": req.PathParameter("namespace"), "name": req.PathParameter("name")})
		}).Doc("Get a report").Operation("report").Writes(map[string]string{}).
			Filter(func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
				resp.AddHeader("X-Filtered", "true")
				chain.ProcessFilter(req, resp)
			}))
		return ws
	}
	routes := &RestfulRoutes{
		Cluster: service("/reports"), Namespaced: service("/reports"),
		Kinds: map[string]*restful.WebService{"Person": service("/report"), "Node": service("/report")},
	}
	kinds := []ManifestVersionKind{
		{Kind: "Person", Plural: "people", Scope: "Namespaced"},
		{Kind: "Node", Scope: "Cluster"},
	}
	manifest := &ManifestData{Group: "example.grafana.app", Versions: []ManifestVersion{
		{Name: "v0", Served: false, Kinds: kinds},
		{Name: "v1", Served: true, Kinds: kinds},
		{Name: "v2", Served: true, Kinds: kinds},
	}}
	ws, err := routes.WebService("v1", manifest)
	require.NoError(t, err)
	require.Len(t, ws.Routes(), 4)
	container := restful.NewContainer()
	container.Add(ws)
	for _, version := range []string{"v1"} {
		for _, tc := range []struct{ path, namespace, name string }{
			{"/reports", "", ""},
			{"/namespaces/team/reports", "team", ""},
			{"/namespaces/team/people/alice/report", "team", "alice"},
			{"/nodes/server/report", "", "server"},
		} {
			t.Run(version+tc.path, func(t *testing.T) {
				response := httptest.NewRecorder()
				container.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/apis/example.grafana.app/"+version+tc.path, nil))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Equal(t, "true", response.Header().Get("X-Filtered"))
				require.JSONEq(t, `{"namespace":"`+tc.namespace+`","name":"`+tc.name+`"}`, response.Body.String())
			})
		}
	}
	// A requested version is mounted once; other versions must not be exposed.
	response := httptest.NewRecorder()
	container.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/apis/example.grafana.app/v2/reports", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
	v2, err := routes.WebService("v2", manifest)
	require.NoError(t, err)
	require.Equal(t, "/apis/example.grafana.app/v2/reports/", v2.Routes()[0].Path)
	_, err = routes.WebService("v0", manifest)
	require.Error(t, err)
	for _, route := range ws.Routes() {
		require.Equal(t, "Get a report", route.Doc)
		require.Equal(t, "report", route.Operation)
		require.NotNil(t, route.WriteSample)
	}
	require.Len(t, ws.Routes()[2].ParameterDocs, 2)
	require.Equal(t, "/reports/", routes.Cluster.Routes()[0].Path)
	require.Equal(t, "/report/", routes.Kinds["Person"].Routes()[0].Path)
	// Repeated construction must not accumulate prefixes or parameters.
	again, err := routes.WebService("v1", manifest)
	require.NoError(t, err)
	require.Equal(t, ws.Routes()[2].Path, again.Routes()[2].Path)
	require.Len(t, again.Routes()[2].ParameterDocs, 2)
}

func TestRestfulRoutesWebServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		routes   *RestfulRoutes
		manifest *ManifestData
	}{
		{"nil routes", nil, &ManifestData{}},
		{"nil manifest", &RestfulRoutes{}, nil},
		{"missing group", &RestfulRoutes{}, &ManifestData{}},
		{"no served versions", &RestfulRoutes{}, &ManifestData{Group: "example.app"}},
		{"unknown kind", &RestfulRoutes{Kinds: map[string]*restful.WebService{"Missing": {}}}, &ManifestData{Group: "example.app", Versions: []ManifestVersion{{Name: "v1", Served: true}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.routes.WebService("v1", tc.manifest)
			require.Error(t, err)
		})
	}
}

func TestRestfulRoutesMountAllKindRoutes(t *testing.T) {
	child := new(restful.WebService).Path("/actions").Produces(restful.MIME_JSON)
	for _, action := range []string{"start", "stop"} {
		child.Route(child.POST("/" + action).To(func(req *restful.Request, resp *restful.Response) {
			_ = resp.WriteEntity(map[string]string{"name": req.PathParameter("name")})
		}))
	}
	// Kind routes can be mounted even when no cluster or namespaced service exists.
	routes := &RestfulRoutes{Kinds: map[string]*restful.WebService{"Widget": child}}
	manifest := &ManifestData{Group: "example.app", Versions: []ManifestVersion{{
		Name: "v1", Served: true,
		Kinds: []ManifestVersionKind{{Kind: "Widget", Scope: "Namespaced"}},
	}}}
	parent, err := routes.WebService("v1", manifest)
	require.NoError(t, err)
	require.Len(t, parent.Routes(), 2)
	container := restful.NewContainer()
	container.Add(parent)
	for i, action := range []string{"start", "stop"} {
		path := "/apis/example.app/v1/namespaces/{namespace}/widgets/{name}/actions/" + action
		require.Equal(t, path, parent.Routes()[i].Path)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost,
			"/apis/example.app/v1/namespaces/team/widgets/example/actions/"+action, nil)
		request.Header.Set("Content-Type", restful.MIME_JSON)
		container.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"name":"example"}`, response.Body.String())
		require.Equal(t, "/actions/"+action, child.Routes()[i].Path)
	}
}
