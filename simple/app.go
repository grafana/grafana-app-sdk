package simple

import (
	"context"
	"fmt"
	"net/http"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var _ resource.AppProvider = &AppProvider{}

// AppProvider is a simple implementation of resource.AppProvider which returns AppManifest when Manifest is called,
// and calls NewAppFunc when NewApp is called.
type AppProvider struct {
	AppManifest resource.AppManifest
	NewAppFunc  func(config resource.AppConfig) (resource.App, error)
}

// Manifest returns the AppManifest in the AppProvider
func (a *AppProvider) Manifest() resource.AppManifest {
	return a.AppManifest
}

// NewApp calls NewAppFunc and returns the result
func (a *AppProvider) NewApp(settings resource.AppConfig) (resource.App, error) {
	return a.NewAppFunc(settings)
}

// NewAppProvider is a convenience method for creating a new AppProvider
func NewAppProvider(manifest resource.AppManifest, newAppFunc func(cfg resource.AppConfig) (resource.App, error)) *AppProvider {
	return &AppProvider{
		AppManifest: manifest,
		NewAppFunc:  newAppFunc,
	}
}

var (
	_ resource.FullApp = &App{}
)

// App is a simple, opinionated implementation of resource.App.
type App struct {
	informerController *operator.InformerController
	operator           *operator.Operator
	clientGenerator    resource.ClientGenerator
	kinds              map[string]AppManagedKind
	internalKinds      map[string]resource.Kind
	cfg                AppConfig
	converters         map[string]k8s.Converter
}

type AppConfig struct {
	Kubeconfig     rest.Config
	InformerConfig AppInformerConfig
	ManagedKinds   []AppManagedKind
}

type AppInformerConfig struct {
	ErrorHandler       func(context.Context, error)
	RetryPolicy        operator.RetryPolicy
	RetryDequeuePolicy operator.RetryDequeuePolicy
}

type AppManagedKind struct {
	Kind              resource.Kind
	Reconciler        operator.Reconciler
	Watcher           operator.ResourceWatcher
	Validator         resource.ValidatingAdmissionController
	Mutator           resource.MutatingAdmissionController
	SubresourceRoutes map[string]func(ctx context.Context, writer http.ResponseWriter, req *resource.SubresourceRequest) error
}

func NewApp(config AppConfig) (*App, error) {
	app := &App{
		informerController: operator.NewInformerController(operator.DefaultInformerControllerConfig()),
		operator:           operator.New(),
		clientGenerator:    k8s.NewClientRegistry(config.Kubeconfig, k8s.DefaultClientConfig()),
		kinds:              make(map[string]AppManagedKind),
		internalKinds:      make(map[string]resource.Kind),
		converters:         make(map[string]k8s.Converter),
		cfg:                config,
	}
	for _, kind := range config.ManagedKinds {
		app.ManageKind(kind, ReconcileOptions{})
	}
	app.operator.AddController(app.informerController)
	return app, nil
}

// ManagedKinds returns a slice of all Kinds managed by this App
func (a *App) ManagedKinds() []resource.Kind {
	kinds := make([]resource.Kind, 0)
	for _, k := range a.kinds {
		kinds = append(kinds, k.Kind)
	}
	return kinds
}

// Runner returns a resource.Runnable() that runs the underlying operator.InformerController and all custom runners
// added via AddRunnable. The returned resource.Runnable also implements metrics.Provider, allowing the caller
// to gather prometheus.Collector objects used by all underlying runners.
func (a *App) Runner() resource.Runnable {
	return a.operator
}

// AddRunnable adds an arbitrary resource.Runnable runner to the App, which will be encapsulated as part of Runner().Run().
// If the provided runner also implements metrics.Provider, PrometheusCollectors() will be called when called on Runner().
func (a *App) AddRunnable(runner resource.Runnable) {
	a.operator.AddController(runner)
}

type ReconcileOptions struct {
	Namespace    string
	LabelFilters []string
	UsePlain     bool
}

func (a *App) ManageKind(kind AppManagedKind, options ReconcileOptions) error {
	a.kinds[gvk(kind.Kind.Group(), kind.Kind.Version(), kind.Kind.Kind())] = kind
	if kind.Reconciler != nil || kind.Watcher != nil {
		client, err := a.clientGenerator.ClientFor(kind.Kind)
		if err != nil {
			return err
		}
		inf, err := operator.NewKubernetesBasedInformerWithFilters(kind.Kind, client, options.Namespace, options.LabelFilters)
		if err != nil {
			return err
		}
		a.informerController.AddInformer(inf, kind.Kind.GroupVersionKind().String())
		if kind.Reconciler != nil {
			reconciler := kind.Reconciler
			if !options.UsePlain {
				op, err := operator.NewOpinionatedReconciler(client, "TODO")
				if err != nil {
					return err
				}
				op.Wrap(kind.Reconciler)
				reconciler = op
			}
			a.informerController.AddReconciler(reconciler, kind.Kind.GroupVersionKind().String())
		}
		if kind.Watcher != nil {
			watcher := kind.Watcher
			if !options.UsePlain {
				op, err := operator.NewOpinionatedWatcherWithFinalizer(kind.Kind, client, func(sch resource.Schema) string {
					return "TODO"
				})
				if err != nil {
					return err
				}
				if cast, ok := kind.Watcher.(syncWatcher); ok {
					op.Wrap(cast, false)
					op.SyncFunc = cast.Sync
				} else {
					op.Wrap(kind.Watcher, true)
				}
				watcher = op
			}
			a.informerController.AddWatcher(watcher, kind.Kind.GroupVersionKind().String())
		}
	}
	return nil
}

func (a *App) RegisterKindConverter(groupKind schema.GroupKind, converter k8s.Converter) {
	a.converters[groupKind.String()] = converter
}

func (a *App) Validate(ctx context.Context, req *resource.AdmissionRequest) error {
	k, ok := a.kinds[gvk(req.Group, req.Version, req.Kind)]
	if !ok {
		// TODO: Default validator
		return nil
	}
	if k.Validator == nil {
		return nil
	}
	return k.Validator.Validate(ctx, req)
}

func (a *App) Mutate(ctx context.Context, req *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
	k, ok := a.kinds[gvk(req.Group, req.Version, req.Kind)]
	if !ok {
		// TODO: Default mutator?
		return nil, nil
	}
	if k.Validator == nil {
		return nil, nil
	}
	return k.Mutator.Mutate(ctx, req)
}

func (a *App) Convert(ctx context.Context, req resource.ConversionRequest) (*resource.RawObject, error) {
	converter, ok := a.converters[req.SourceGVK.GroupKind().String()]
	if !ok {
		// Default conversion?
		return nil, fmt.Errorf("no converter")
	}
	srcAPIVersion, _ := req.SourceGVK.ToAPIVersionAndKind()
	dstAPIVersion, _ := req.TargetGVK.ToAPIVersionAndKind()
	converted, err := converter.Convert(k8s.RawKind{
		Kind:       req.SourceGVK.Kind,
		APIVersion: srcAPIVersion,
		Group:      req.SourceGVK.Group,
		Version:    req.SourceGVK.Version,
		Raw:        req.Raw.Raw,
	}, dstAPIVersion)
	return &resource.RawObject{
		Raw: converted,
	}, err
}

func (a *App) CallSubresource(ctx context.Context, writer http.ResponseWriter, req *resource.SubresourceRequest) error {
	k, ok := a.kinds[gvk(req.ResourceIdentifier.Group, req.ResourceIdentifier.Version, req.ResourceIdentifier.Kind)]
	if !ok {
		writer.WriteHeader(http.StatusNotFound)
		return nil
	}
	handler, ok := k.SubresourceRoutes[req.SubresourcePath]
	if !ok {
		writer.WriteHeader(http.StatusNotFound)
		return nil
	}
	return handler(ctx, writer, req)
}

type syncWatcher interface {
	operator.ResourceWatcher
	Sync(ctx context.Context, object resource.Object) error
}

func gvk(group, version, kind string) string {
	return fmt.Sprintf("%s/%s/%s", group, version, kind)
}
