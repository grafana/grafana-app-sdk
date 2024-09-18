package simple

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
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
	AppManifest       app.Manifest
	AppSpecificConfig app.SpecificConfig
	NewAppFunc        func(config app.Config) (app.App, error)
}

// Manifest returns the AppManifest in the AppProvider
func (p *AppProvider) Manifest() app.Manifest {
	return p.AppManifest
}

func (p *AppProvider) SpecificConfig() app.SpecificConfig {
	return p.AppSpecificConfig
}

// NewApp calls NewAppFunc and returns the result
func (p *AppProvider) NewApp(settings app.Config) (app.App, error) {
	return p.NewAppFunc(settings)
}

// NewAppProvider is a convenience method for creating a new AppProvider
func NewAppProvider(manifest app.Manifest, cfg app.SpecificConfig, newAppFunc func(cfg app.Config) (app.App, error)) *AppProvider {
	return &AppProvider{
		AppManifest:       manifest,
		AppSpecificConfig: cfg,
		NewAppFunc:        newAppFunc,
	}
}

var (
	_ app.App = &App{}
)

// App is a simple, opinionated implementation of app.App.
// It must be created with NewApp to be valid.
type App struct {
	informerController *operator.InformerController
	runner             *app.MultiRunner
	clientGenerator    resource.ClientGenerator
	kinds              map[string]AppManagedKind
	internalKinds      map[string]resource.Kind
	cfg                AppConfig
	converters         map[string]Converter
}

// AppConfig is the configuration used by App
type AppConfig struct {
	Name           string
	KubeConfig     rest.Config
	InformerConfig AppInformerConfig
	ManagedKinds   []AppManagedKind
	UnmanagedKinds []AppUnmanagedKind
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
	// CustomRoutes are an optional map of subresource paths to a route handler.
	// If supported by the runner, calls to these subresources on this particular version will call this handler.
	CustomRoutes []AppCustomRouteHandler
	// ReconcileOptions are the options to use for running the Reconciler or Watcher for the Kind, if one exists.
	ReconcileOptions BasicReconcileOptions
}

type AppCustomRouteHandler struct {
	Path    string
	Methods []string
	Handler func(context.Context, *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error)
}

// AppUnmanagedKind is a Kind which an App does not manage, but still may want to watch or reconcile as part of app functionality
type AppUnmanagedKind struct {
	// Kind is the resource.Kind the App has an interest in, but does not manage.
	Kind resource.Kind
	// Reconciler is an optional reconciler to run for this Kind.
	Reconciler operator.Reconciler
	// Watcher is an optional Watcher to run for this Kind.
	Watcher operator.ResourceWatcher
	// ReconcileOptions are the options to use for running the Reconciler or Watcher for the Kind, if one exists.
	ReconcileOptions BasicReconcileOptions
}

// BasicReconcileOptions are settings for the ListWatch and informer setup for a reconciliation loop
type BasicReconcileOptions struct {
	// Namespace is the namespace to use in the ListWatch request
	Namespace string
	// LabelFilters are any label filters to apply to the ListWatch request
	LabelFilters []string
	// FieldSelectors are any field selector filters to apply to the ListWatch request
	FieldSelectors []string
	// UsePlain can be set to true to avoid wrapping the Reconciler or Watcher in its Opinionated variant.
	UsePlain bool
}

// NewApp creates a new instance of App, managing the kinds provided in AppConfig.ManagedKinds.
// AppConfig MUST contain a valid KubeConfig to be valid.
// Watcher/Reconciler error handling, retry, and dequeue logic can be managed with AppConfig.InformerConfig.
func NewApp(config AppConfig) (*App, error) {
	a := &App{
		informerController: operator.NewInformerController(operator.DefaultInformerControllerConfig()),
		runner:             app.NewMultiRunner(),
		clientGenerator:    k8s.NewClientRegistry(config.KubeConfig, k8s.DefaultClientConfig()),
		kinds:              make(map[string]AppManagedKind),
		internalKinds:      make(map[string]resource.Kind),
		converters:         make(map[string]Converter),
		cfg:                config,
	}
	for _, kind := range config.ManagedKinds {
		err := a.manageKind(kind)
		if err != nil {
			return nil, err
		}
	}
	for _, kind := range config.UnmanagedKinds {
		err := a.watchKind(kind)
		if err != nil {
			return nil, err
		}
	}
	for gk, converter := range config.Converters {
		a.RegisterKindConverter(gk, converter)
	}
	a.runner.AddRunnable(&k8sRunnable{
		runner: a.informerController,
	})
	return a, nil
}

// ValidateManifest can be called with app.ManifestData to validate that the current configuration and managed kinds
// fully cover the kinds and capabilities in the provided app.ManifestData. If the provided app.ManifestData
// contains a kind or a capability for a kind/version that is not covered by the app's currently managed kinds,
// an error will be returned. If the app's current managed kinds cover more than the provided app.ManifestData
// indicates, no error will be returned.
// This method can be used after initializing an app to verify it matches the loaded app.ManifestData from the app runner.
func (a *App) ValidateManifest(manifest app.ManifestData) error {
	for _, k := range manifest.Kinds {
		if _, ok := a.converters[schema.GroupKind{Group: manifest.Group, Kind: k.Kind}.String()]; !ok && k.Conversion {
			return fmt.Errorf("kind %s has conversion enabled but no converter is registered", k.Kind)
		}
		for _, v := range k.Versions {
			kind, ok := a.kinds[gvk(manifest.Group, v.Name, k.Kind)]
			if !ok {
				return fmt.Errorf("kind %s/%s exists in manifest but is not managed by the app", k.Kind, v.Name)
			}
			if v.Admission.SupportsAnyValidation() && kind.Validator == nil {
				return fmt.Errorf("kind %s/%s supports validation but has no validator", k.Kind, v.Name)
			}
			if v.Admission.SupportsAnyMutation() && kind.Mutator == nil {
				return fmt.Errorf("kind %s/%s supports mutation but has no mutator", k.Kind, v.Name)
			}
		}
	}
	return nil
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
	return a.runner
}

// AddRunnable adds an arbitrary resource.Runnable runner to the App, which will be encapsulated as part of Runner().Run().
// If the provided runner also implements metrics.Provider, PrometheusCollectors() will be called when called on Runner().
func (a *App) AddRunnable(runner app.Runnable) {
	a.runner.AddRunnable(runner)
}

// manageKind introduces a new kind to manage.
func (a *App) manageKind(kind AppManagedKind) error {
	a.kinds[gvk(kind.Kind.Group(), kind.Kind.Version(), kind.Kind.Kind())] = kind
	// If there are custom routes, validate them
	if len(kind.CustomRoutes) > 0 {
		pathMethods := make(map[string]struct{})
		for _, route := range kind.CustomRoutes {
			if len(route.Methods) == 0 {
				return fmt.Errorf("custom route cannot have no Methods")
			}
			for _, method := range route.Methods {
				pm := fmt.Sprintf("%s:%s", strings.ToUpper(method), route.Path)
				if _, ok := pathMethods[pm]; ok {
					return fmt.Errorf("custom route '%s' already has a handler for method '%s'", route.Path, method)
				}
				pathMethods[pm] = struct{}{}
			}
		}
	}
	if kind.Reconciler != nil || kind.Watcher != nil {
		return a.watchKind(AppUnmanagedKind{
			Kind:             kind.Kind,
			Reconciler:       kind.Reconciler,
			Watcher:          kind.Watcher,
			ReconcileOptions: kind.ReconcileOptions,
		})
	}
	return nil
}

func (a *App) watchKind(kind AppUnmanagedKind) error {
	if kind.Reconciler != nil && kind.Watcher != nil {
		return fmt.Errorf("please provide either Watcher or Reconciler, not both")
	}
	if kind.Reconciler != nil || kind.Watcher != nil {
		client, err := a.clientGenerator.ClientFor(kind.Kind)
		if err != nil {
			return err
		}
		inf, err := operator.NewKubernetesBasedInformerWithFilters(kind.Kind, client, operator.ListWatchOptions{
			Namespace:      kind.ReconcileOptions.Namespace,
			LabelFilters:   kind.ReconcileOptions.LabelFilters,
			FieldSelectors: kind.ReconcileOptions.FieldSelectors,
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
			if !kind.ReconcileOptions.UsePlain {
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
			if !kind.ReconcileOptions.UsePlain {
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

// PrometheusCollectors implements metrics.Provider and returns prometheus collectors used by the app for exposing metrics
func (a *App) PrometheusCollectors() []prometheus.Collector {
	// TODO: other collectors?
	return a.runner.PrometheusCollectors()
}

// Validate implements app.App and handles Validating Admission Requests
func (a *App) Validate(ctx context.Context, req *resource.AdmissionRequest) error {
	k, ok := a.kinds[gvk(req.Group, req.Version, req.Kind)]
	if !ok {
		// TODO: Default validator instead of ErrNotImplemented?
		return app.ErrNotImplemented
	}
	if k.Validator == nil {
		return app.ErrNotImplemented
	}
	return k.Validator.Validate(ctx, req)
}

// Mutate implements app.App and handles Mutating Admission Requests
func (a *App) Mutate(ctx context.Context, req *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
	k, ok := a.kinds[gvk(req.Group, req.Version, req.Kind)]
	if !ok {
		// TODO: Default mutator instead of ErrNotImplemented?
		return nil, app.ErrNotImplemented
	}
	if k.Mutator == nil {
		return nil, app.ErrNotImplemented
	}
	return k.Mutator.Mutate(ctx, req)
}

// Convert implements app.App and handles resource conversion requests
func (a *App) Convert(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
	converter, ok := a.converters[req.SourceGVK.GroupKind().String()]
	if !ok {
		// Default conversion?
		return nil, app.ErrNotImplemented
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

// CallResourceCustomRoute implements app.App and handles custom resource route requests
func (a *App) CallResourceCustomRoute(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
	k, ok := a.kinds[gvk(req.ResourceIdentifier.Group, req.ResourceIdentifier.Version, req.ResourceIdentifier.Kind)]
	if !ok {
		// TODO: still return the not found, or just return NotImplemented?
		return nil, app.ErrCustomRouteNotFound
	}
	for _, handler := range k.CustomRoutes {
		if handler.Path == req.SubresourcePath {
			for _, method := range handler.Methods {
				if strings.EqualFold(method, req.Method) {
					return handler.Handler(ctx, req)
				}
			}
		}
	}
	return nil, app.ErrCustomRouteNotFound
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

type k8sRunner interface {
	Run(<-chan struct{}) error
}

type k8sRunnable struct {
	runner k8sRunner
}

func (k *k8sRunnable) Run(ctx context.Context) error {
	return k.runner.Run(ctx.Done())
}
