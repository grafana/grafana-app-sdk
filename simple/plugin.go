package simple

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"

	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-app-sdk/resource"
)

type PluginError struct {
	Error string `json:"error"`
}

var DefaultPluginErrorHandler PluginErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
	// TODO
}

type PluginErrorHandler func(http.ResponseWriter, *http.Request, error)

// Plugin is a simple variant of a grafana-plugin-sdk-go CallResource-compatible plugin which does resource routing.
// It exposes Create/Read/Update/Delete/List endpoints for any resources added with HandleCRUDL.
type Plugin struct {
	ErrorHandler PluginErrorHandler
	router       *router.JSONRouter
}

func (p *Plugin) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return p.router.CallResource(ctx, req, sender)
}

func (p *Plugin) HandleCRUDL(schema resource.Schema, handlers router.JSONResourceHandler, pathPrefix string) *router.JSONRouter {
	var sr *router.JSONRouter
	if pathPrefix != "" {
		p.router.Subroute(fmt.Sprintf("%s/%s", pathPrefix, schema.Version()))
	} else {
		p.router.Subroute(schema.Version())
	}
	sr.HandleResource(schema.Plural(), handlers)
	return sr
}

func NewJSONResourceHandler[APIForm any, StoreForm resource.Object](schema resource.Schema, converter Converter[APIForm, StoreForm], store resource.TypedStore[StoreForm], ns string) router.JSONResourceHandler {
	return router.JSONResourceHandler{
		Create: func(ctx context.Context, request router.JSONRequest) (router.JSONResponse, error) {
			ctx, span := tracing.DefaultTracer().Start(ctx, fmt.Sprintf("%s-create", schema.Plural()))
			defer span.End()
			body, err := io.ReadAll(request.Body)
			if err != nil {
				// TODO
				return nil, err
			}
			// Read object as APIForm
			// TODO: use a parser instead of JSON?
			obj := new(APIForm)
			err = json.Unmarshal(body, obj)
			if err != nil {
				// TODO
				return nil, err
			}
			// TODO: validate--or leave that to the operator's validation?
			// Convert
			storeForm, err := converter.ToStore(ctx, *obj)
			if err != nil {
				// TODO
				return nil, err
			}
			// TODO: preprocess? Or is that too much for "simple"?
			return store.Add(ctx, storeForm)
		},
		Read: func(ctx context.Context, request router.JSONRequest) (router.JSONResponse, error) {
			ctx, span := tracing.DefaultTracer().Start(ctx, fmt.Sprintf("%s-get", schema.Plural()))
			defer span.End()
			obj, err := store.Get(ctx, resource.Identifier{
				Namespace: ns,
				Name:      request.Vars.MustGet("name"),
			})
			if err != nil {
				return nil, err
			}
			conv, err := converter.ToAPI(ctx, obj)
			return conv, err
		},
		Update: func(ctx context.Context, request router.JSONRequest) (router.JSONResponse, error) {
			return nil, nil
		},
		Delete: func(ctx context.Context, request router.JSONRequest) (router.JSONResponse, error) {
			return nil, nil
		},
		List: func(ctx context.Context, request router.JSONRequest) (router.JSONResponse, error) {
			return nil, nil
		},
	}
}

type Converter[API any, Store resource.Object] interface {
	ToStore(ctx context.Context, apiObject API) (Store, error)
	ToAPI(ctx context.Context, storeObject Store) (API, error)
}

// NoOpConverter is a Converter which has both API and store forms as the same resource.Object,
// so simply returns the same object for ToStore and ToAPI calls
type NoOpConverter[SingleForm resource.Object] struct{}

// ToStore returns the apiObject unchanged
func (*NoOpConverter[SingleForm]) ToStore(_ context.Context, apiObject SingleForm) (SingleForm, error) {
	return apiObject, nil
}

// ToAPI returns the storeObject unchanged
func (*NoOpConverter[SingleForm]) ToAPI(_ context.Context, storeObject SingleForm) (SingleForm, error) {
	return storeObject, nil
}

// Compile-time interface compliance check
var _ Converter[resource.Object, resource.Object] = &NoOpConverter[resource.Object]{}
