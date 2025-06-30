package plugin

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/plugin/kubeconfig"
	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/issue-tracker-project/pkg/plugin/secure"
)

type Service interface {
	GetIssueService(context.Context) (IssueService, error)
}

// Plugin is the backend plugin
type Plugin struct {
	router    *router.JSONRouter
	namespace string
	service   Service
}

// Start has the plugin's router start listening over gRPC, and blocks until an unrecoverable error occurs
func (p *Plugin) Start() error {
	return p.router.ListenAndServe()
}

// CallResource allows Plugin to implement grafana-plugin-sdk-go/backend/instancemgmt.Instance for an App plugin,
// Which allows it to be used with grafana-plugin-sdk-go/backend/app.Manage.
// CallResource downstreams all CallResource requests to the router's handler
func (p *Plugin) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return p.router.CallResource(ctx, req, sender)
}

func New(namespace string, service Service) (*Plugin, error) {
	p := &Plugin{
		router:    router.NewJSONRouter(),
		namespace: namespace,
		service:   service,
	}

	p.router.Use(
		router.NewTracingMiddleware(otel.GetTracerProvider().Tracer("tracing-middleware")),
		router.NewLoggingMiddleware(logging.DefaultLogger),
		kubeconfig.LoadingMiddleware(),
		router.MiddlewareFunc(secure.Middleware))

	// V1 Routes
	v1Subrouter := p.router.Subroute("v1/")

	// Issue subrouter
	issueSubrouter := v1Subrouter.Subroute("issues/")
	v1Subrouter.Handle("issues", p.handleIssueList, http.MethodGet)
	v1Subrouter.HandleWithCode("issues", p.handleIssueCreate, http.StatusCreated, http.MethodPost)
	issueSubrouter.Handle("{name}", p.handleIssueGet, http.MethodGet)
	issueSubrouter.Handle("{name}", p.handleIssueUpdate, http.MethodPut)
	issueSubrouter.HandleWithCode("{name}", p.handleIssueDelete, http.StatusNoContent, http.MethodDelete)

	return p, nil
}

type errWithStatusCode interface {
	error
	StatusCode() int
}
