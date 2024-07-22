package operator

import (
	"context"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
)

type PluginController interface {
	RunPlugin()
}

type OperatorConfig struct {
	Logger logging.Logger
}

func NewOperator(cfg OperatorConfig) (*Operator, error) {
	o := Operator{
		logger: plugin.NewLogger(cfg.Logger),
	}
}

type Operator struct {
	operator.Operator

	logger logging.Logger

	PluginID string
}

func (o *Operator) AddController(c operator.Controller) {

}

func (o *Operator) Run() {
	app.Manage(o.PluginID, o.instanceFactoryFunc, app.ManageOpts{})
}

func (o *Operator) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {

}

func (o *Operator) instanceFactoryFunc(ctx context.Context, settings backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	// TODO: need to get kubeconfig from MT settings?
}
