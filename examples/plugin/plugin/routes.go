package plugin

import (
	"net/http"

	"github.com/emicklei/go-restful/v3"
	authlib "github.com/grafana/authlib/types"

	"github.com/grafana/grafana-app-sdk/resource"
)

// Dummy is a sample response type used to generate an OpenAPI schema.
type Dummy struct {
	ID   string                     `json:"id" description:"identifier of the thing"`
	Auth any                        `json:"auth,omitempty" description:"authentication information (from context)"`
	Info *resource.RouteRequestInfo `json:"info" description:"request info"`
}

// ProvideRoutes describes the example plugin callbacks and their operations.
func (*ManagedApp) ProvideRoutes() (*restful.WebService, error) {
	prefix := "/apis/group/v1"
	ws := new(restful.WebService)
	ws.Path(prefix).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	// Cluster Scoped routes setup
	ws.Route(ws.GET("/foo").To(findThings).
		Operation("getFoo").
		Doc("get foo").
		Param(ws.QueryParameter("input", "query").DataType("string")).
		AddExtension("x-grafana-requires-role", "viewer").
		Writes(Dummy{}).
		Returns(http.StatusOK, "cluster request for foo", Dummy{}))

	// POST overrides the shared path role added by customizeRouteOpenAPI.
	// This example echoes input; it does not persist a new resource.
	ws.Route(ws.POST("/foo").To(findThings).
		Operation("postFoo").
		Doc("post foo").
		AddExtension("x-grafana-requires-role", "editor").
		Param(ws.QueryParameter("input", "query").DataType("string")).
		Writes(Dummy{}).
		Returns(http.StatusOK, "cluster request for foo", Dummy{}))

	// The catch-all captures slashes, e.g. /foo/a/b -> PathParameter("path") == "a/b".
	// The final parameter name path preserves catch-all behavior in OpenAPI.
	ws.Route(ws.GET("/foo/{path:*}").To(findThings).
		Operation("getFooPath").
		Param(ws.PathParameter("path", "Remaining path, including slashes").DataType("string")).
		Doc("cluster request matching any path").
		Param(ws.QueryParameter("input", "query").DataType("string")).
		AddExtension("x-grafana-requires-role", "viewer").
		Writes(Dummy{}).
		Returns(http.StatusOK, "cluster request with", Dummy{}))

	// Namespaced Scoped routes setup
	ws.Path(prefix + "/namespaces/{namespace}/things")
	ws.Route(ws.GET("/bar").To(findThings).
		Operation("getBar").
		Doc("get bar").
		Writes(Dummy{}).
		Returns(http.StatusOK, "The requested namespaced thing", Dummy{}))

	// someday... the kinds flavor

	return ws, nil
}

func findThings(request *restful.Request, response *restful.Response) {
	ctx := request.Request.Context()
	auth, _ := authlib.AuthInfoFrom(ctx)

	dummy := Dummy{
		ID:   request.QueryParameter("input"),
		Auth: auth,
		Info: resource.RouteRequestInfoFrom(ctx),
	}

	_ = response.WriteEntity(dummy)
}
