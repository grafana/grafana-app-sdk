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

// handlePing is an example HTTP GET resource that returns a {"message": "ok"} JSON response.
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
		AddExtension("x-grafana-requires-role", "some-role").
		Param(ws.QueryParameter("input", "query").DataType("string")).
		Writes(Dummy{}).
		Returns(http.StatusOK, "cluster request for foo", Dummy{}))

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
