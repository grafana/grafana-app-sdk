package simple

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

var _ app.Provider = &AppProvider{}

type Converter k8s.Converter

// AppProvider is a simple implementation of app.Provider which returns AppManifest when Manifest is called,
// and calls NewAppFunc when NewApp is called.
type AppProvider struct {
	AppManifest app.Manifest
	NewAppFunc  func(config app.Config) (app.App, error)
}

// Manifest returns the AppManifest in the AppProvider
func (a *AppProvider) Manifest() app.Manifest {
	return a.AppManifest
}

// NewApp calls NewAppFunc and returns the result
func (a *AppProvider) NewApp(settings app.Config) (app.App, error) {
	return a.NewAppFunc(settings)
}

// NewAppProvider is a convenience method for creating a new AppProvider
func NewAppProvider(manifest app.Manifest, newAppFunc func(cfg app.Config) (app.App, error)) *AppProvider {
	return &AppProvider{
		AppManifest: manifest,
		NewAppFunc:  newAppFunc,
	}
}

var (
	_ app.App = &App{}
)

// App is a simple, opinionated implementation of resource.App.
// It must be created with NewApp to be valid.
type App struct {
	informerController *operator.InformerController
	operator           *operator.Operator
	clientGenerator    resource.ClientGenerator
	kinds              map[string]AppManagedKind
	internalKinds      map[string]resource.Kind
	cfg                AppConfig
	converters         map[string]Converter
}

// AppConfig is the configuration used by App
type AppConfig struct {
	Name           string
	Kubeconfig     rest.Config
	InformerConfig AppInformerConfig
	ManagedKinds   []AppManagedKind
	Converters     map[schema.GroupKind]Converter
}

// AppInformerConfig contains configuration for the App's internal operator.InformerController
type AppInformerConfig struct {
	ErrorHandler       func(context.Context, error)
	RetryPolicy        operator.RetryPolicy
	RetryDequeuePolicy operator.RetryDequeuePolicy
	FinalizerSupplier  operator.FinalizerSupplier
}

// AppManagedKind is a Kind and associated functionality used by an App.
type AppManagedKind struct {
	// Kind is the resource.Kind being managed. This is equivalent to a kubernetes GroupVersionKind
	Kind resource.Kind
	// Reconciler is an optional reconciler to run for this Kind. Only one version of a Kind should have a Reconciler,
	// otherwise, duplicate events will be received.
	Reconciler operator.Reconciler
	// Watcher is an optional Watcher to run for this Kind. Only one version of a Kind should have a Watcher,
	// otherwise, duplicate events will be received.
	Watcher operator.ResourceWatcher
	// Validator is an optional ValidatingAdmissionController for the Kind. It will be run only for validation
	// of this specific version.
	Validator resource.ValidatingAdmissionController
	// Mutator is an optional MutatingAdmissionController for the Kind. It will be run only for mutation
	// of this specific version.
	Mutator resource.MutatingAdmissionController
	// SubresourceRoutes are an optional map of subresource paths to a route handler.
	// If supported by the runner, calls to these subresources on this particular version will call this handler.
	SubresourceRoutes map[string]func(ctx context.Context, writer http.ResponseWriter, req *app.SubresourceRequest) error
}

// NewApp creates a new instance of App, managing the kinds provided in AppConfig.ManagedKinds.
// AppConfig MUST contain a valid KubeConfig to be valid. Kinds can be managed by the app either with
// App.ManageKind or via AppConfig.ManagedKinds.
// Watcher/Reconciler error handling, retry, and dequeue logic can be managed with AppConfig.InformerConfig.
func NewApp(config AppConfig) (*App, error) {
	a := &App{
		informerController: operator.NewInformerController(operator.DefaultInformerControllerConfig()),
		operator:           operator.New(),
		clientGenerator:    k8s.NewClientRegistry(config.Kubeconfig, k8s.DefaultClientConfig()),
		kinds:              make(map[string]AppManagedKind),
		internalKinds:      make(map[string]resource.Kind),
		converters:         make(map[string]Converter),
		cfg:                config,
	}
	for _, kind := range config.ManagedKinds {
		err := a.ManageKind(kind, ReconcileOptions{})
		if err != nil {
			return nil, err
		}
	}
	for gk, converter := range config.Converters {
		a.converters[gk.String()] = converter
	}
	a.operator.AddController(a.informerController)
	return a, nil
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
func (a *App) Runner() app.Runnable {
	return a.operator
}

// AddRunnable adds an arbitrary resource.Runnable runner to the App, which will be encapsulated as part of Runner().Run().
// If the provided runner also implements metrics.Provider, PrometheusCollectors() will be called when called on Runner().
func (a *App) AddRunnable(runner app.Runnable) {
	a.operator.AddController(runner)
}

// ReconcileOptions are settings for the reconciliation loop
type ReconcileOptions struct {
	// Namespace is the namespace to use in the ListWatch request
	Namespace string
	// LabelFilters are any label filters to apply to the ListWatch request
	LabelFilters []string
	// FieldSelectors are any field selector filters to apply to the ListWatch request
	FieldSelectors []string
	// UsePlain can be set to true to avoid wrapping the Reconciler or Watcher in its Opinionated variant.
	UsePlain bool
}

// ManageKind introduces a new kind to manage.
func (a *App) ManageKind(kind AppManagedKind, options ReconcileOptions) error {
	a.kinds[gvk(kind.Kind.Group(), kind.Kind.Version(), kind.Kind.Kind())] = kind
	if kind.Reconciler != nil || kind.Watcher != nil {
		client, err := a.clientGenerator.ClientFor(kind.Kind)
		if err != nil {
			return err
		}
		inf, err := operator.NewKubernetesBasedInformerWithFilters(kind.Kind, client, operator.ListWatchOptions{
			Namespace:      options.Namespace,
			LabelFilters:   options.LabelFilters,
			FieldSelectors: options.FieldSelectors,
		})
		if err != nil {
			return err
		}
		err = a.informerController.AddInformer(inf, kind.Kind.GroupVersionKind().String())
		if err != nil {
			return fmt.Errorf("could not add informer to controller: %v", err)
		}
		if kind.Reconciler != nil {
			reconciler := kind.Reconciler
			if !options.UsePlain {
				op, err := operator.NewOpinionatedReconciler(client, a.getFinalizer(kind.Kind))
				if err != nil {
					return err
				}
				op.Wrap(kind.Reconciler)
				reconciler = op
			}
			err = a.informerController.AddReconciler(reconciler, kind.Kind.GroupVersionKind().String())
			if err != nil {
				return fmt.Errorf("could not add reconciler to controller: %v", err)
			}
		}
		if kind.Watcher != nil {
			watcher := kind.Watcher
			if !options.UsePlain {
				op, err := operator.NewOpinionatedWatcherWithFinalizer(kind.Kind, client, a.getFinalizer)
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
			err = a.informerController.AddWatcher(watcher, kind.Kind.GroupVersionKind().String())
			if err != nil {
				return fmt.Errorf("could not add watcher to controller: %v", err)
			}
		}
	}
	return nil
}

// RegisterKindConverter adds a converter for a GroupKind, which will then be processed on Convert calls
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

func (a *App) Convert(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
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
	return &app.RawObject{
		Raw: converted,
	}, err
}

func (a *App) CallSubresource(ctx context.Context, writer http.ResponseWriter, req *app.SubresourceRequest) error {
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

func (a *App) getFinalizer(sch resource.Schema) string {
	if a.cfg.InformerConfig.FinalizerSupplier != nil {
		return a.cfg.InformerConfig.FinalizerSupplier(sch)
	}
	if a.cfg.Name != "" {
		return fmt.Sprintf("%s-%s-finalizer", a.cfg.Name, sch.Plural())
	}
	return fmt.Sprintf("%s-finalizer", sch.Plural())
}

type syncWatcher interface {
	operator.ResourceWatcher
	Sync(ctx context.Context, object resource.Object) error
}

func gvk(group, version, kind string) string {
	return fmt.Sprintf("%s/%s/%s", group, version, kind)
}
