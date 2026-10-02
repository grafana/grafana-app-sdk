package plugin

import (
	"net/http"

	"github.com/emicklei/go-restful/v3"

	"github.com/grafana/grafana-app-sdk/app"
)

// Dummy is a sample response type used to generate an OpenAPI schema.
type Dummy struct {
	ID   string `json:"id" description:"identifier of the thing"`
	Name string `json:"name" description:"name of the thing" default:"john"`
	Age  int    `json:"age" description:"age of the thing" default:"21"`
}

// handlePing is an example HTTP GET resource that returns a {"message": "ok"} JSON response.
func (*ManagedApp) ProvideRoutes(version string) (*app.RestfulRoutes, error) {
	routes := &app.RestfulRoutes{}

	// Cluster Scoped routes setup
	ws := new(restful.WebService)
	ws.Path("/things").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("/{thing-id}").To(findThings).
		Operation("getClusterThing").
		Doc("get a thing").
		AddExtension("x-grafana-requires-role", "some-role").
		Param(ws.PathParameter("thing-id", "identifier of the thing").DataType("string")).
		Writes(Dummy{}).
		Returns(http.StatusOK, "The requested cluster thing", Dummy{}))
	routes.Cluster = ws

	// Namespaced Scoped routes setup
	ws = new(restful.WebService)
	ws.Path("/things").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("/{thing-id}").To(findThings).
		Operation("getNamespacedThing").
		Doc("get a thing").
		Param(ws.PathParameter("thing-id", "identifier of the thing").DataType("string")).
		Writes(Dummy{}).
		Returns(http.StatusOK, "The requested namespaced thing", Dummy{}))
	routes.Namespaced = ws

	// someday... the kinds flavor

	return routes, nil
}

func findThings(request *restful.Request, response *restful.Response) {
	id := request.PathParameter("thing-id")
	_ = response.WriteEntity(Dummy{ID: id, Name: "dumb dumb (cluster)", Age: 49})
}
