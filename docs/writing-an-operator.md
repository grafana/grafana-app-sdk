# Writing an Operator

An "operator" is a generic term for an application or process which watches for events on one or more kinds and takes action based on received events. Within the `grafana-app-sdk`, an operator refers to either the `operator.Operator`/`simple.Operator` type (which is a process that runs one or more other processes), or a standalone application which performs the tasks of an operator (these two definitions often overlap, with the standalone application using `operator.Operator` or `simple.Operator` to perform said tasks). An operator application may also supply one or more webhooks for a kind, for admission and/or version conversion.

The easiest way to get started with an operator is using the generated operator code from `grafana-app-sdk project component add operator`. This will generate boilerplate operator code for you to boostrap your operator with, using the `simple` package. However, we will break down writing a similar operator using the `simple` package here, and then a more complex one using the `operator` package.

If you have not yet read through the [Issue Tracker Tutorial](./tutorials/issue-tracker/README.md), there is a [section on writing an operator](./tutorials/issue-tracker/07-operator-watcher.md)

## Simple Operator

The `simple` package offers an easy way to set up an operator, and is useful for most workflows. It doesn't expose the internals, and offers the ability to set up watchers, reconcilers, and webhooks either in an initial config or post-creation. It also has built-in observability with metrics, logs, and traces. Creating a new simple operator uses the `simple.NewOperator` method:
```go
op, err := simple.NewOperator(simple.OperatorConfig{
    Name: "my-operator", // Name used to uniquely identify the operator, and use used for finalizers
    KubeConfig: kubeConfig, // Kubernetes rest.Config used for the operator to talk to the kubernetes API server
})
```
And that's all you need to create a simple operator. If you want to set up metrics or tracing it's another config option:
```go
op, err := simple.NewOperator(simple.OperatorConfig{
    Name: "my-operator", // Name used to uniquely identify the operator, and use used for finalizers
    KubeConfig: kubeConfig, // Kubernetes rest.Config used for the operator to talk to the kubernetes API server
    Metrics: simple.MetricsConfig{ 
        Enabled: true, // Sets up a prometheus scrape endpoint at /metrics
    },
    Tracing: simple.TracingConfig{
        Enabled: true, // Sets up tracing
        OpenTelemetryConfig: simple.OpenTelemetryConfig{ // Configuration for the OTel collector
            Host:        cfg.OTelConfig.Host,
            Port:        cfg.OTelConfig.Port,
            ConnType:    OTelConnTypeGRPC,
            ServiceName: "test",
        },
    },
})
```
If you have processes outside of the operator that emit metrics that you want to expose via the operator's `/metrics` endpoint, you can use the registerer in your `MetricsConfig` to register them (this defaults to the prometheus default registerer), or call `op.RegisterMetricsCollectors` to register your prometheus collectors with the operator.

You can add a watcher or reconciler for one or more kinds by calling `WatchKind` or `ReconcileKind` respectively. These methods will automatically wrap your watcher or reconciler in their opinionated variant
```go
err = op.WatchKind(mykindv1.Kind(), &MyKindWatcher{}, simple.ListWatchOptions{
    Namespace: resource.NamespaceAll,
    LabelFilters: []string{"foo=bar"},
})
```
```go
err = op.ReconcileKind(mykindv1.Kind(), &MyKindReconciler{}, simple.ListWatchOptions{
    Namespace: resource.NamespaceAll,
    LabelFilters: []string{"foo=bar"},
})
```

Finally, you can do admission control and version conversion via webhooks using either the config or dedicated methods. To use the `ValidateKind`, `MutateKind`, and `ConvertKind` methods on the operator, you must have webhooks enabled in the config (otherwise these methods will return an error). This is done via the `Webhooks` section of the `OperatorConfig`:
```go
op, err := simple.NewOperator(simple.OperatorConfig{
    Name: "my-operator", // Name used to uniquely identify the operator, and use used for finalizers
    KubeConfig: kubeConfig, // Kubernetes rest.Config used for the operator to talk to the kubernetes API server
    Webhooks: simple.WebhookConfig{
        Enabled: true,
        Port:    8443,
        TLSConfig: k8s.TLSConfig{
            CertPath: cfg.WebhookServer.TLSCertPath,
            KeyPath:  cfg.WebhookServer.TLSKeyPath,
        },
    },
})
```
Optionally, you can specify a default mutating and validating admission controller to use if the `/mutate` or `/validate` endpoints are hit for a kind you haven't added a mutator or validator for:
```go
op, err := simple.NewOperator(simple.OperatorConfig{
    Name: "my-operator", // Name used to uniquely identify the operator, and use used for finalizers
    KubeConfig: kubeConfig, // Kubernetes rest.Config used for the operator to talk to the kubernetes API server
    Webhooks: simple.WebhookConfig{
        Enabled: true,
        Port:    8443,
        TLSConfig: k8s.TLSConfig{
            CertPath: cfg.WebhookServer.TLSCertPath,
            KeyPath:  cfg.WebhookServer.TLSKeyPath,
        },
        DefaultValidator: &MyGenericValidator{},
        DefaultMutator: &MyGenericMutator{},
    },
})
```
kind-specific mutator and validators can be added to the config, or via the `MutateKind` and `ValidateKind` methods. Version conversion can likewise be added either to the config, or with the `ConvertKind` method. Note that conversion doesn't use a `resource.Kind`, instead accepting a `metav1.GroupKind`, as version conversion is tied to every version of a kind, and uses one function across all of them.

Finally, you can set up custom error handling for failed watch/reconcile events with the `ErrorHandler` config field.

A `simple.Operator` that uses all functionality might look something like this:
```go
myKind := mykindv1.Kind()
otherKind := otherkindv1.Kind()
op, err := simple.NewOperator(simple.OperatorConfig{
    Name: "my-operator", // Name used to uniquely identify the operator, and use used for finalizers
    KubeConfig: kubeConfig, // Kubernetes rest.Config used for the operator to talk to the kubernetes API server
    Webhooks: simple.WebhookConfig{
        Enabled: true,
        Port:    8443,
        TLSConfig: k8s.TLSConfig{
            CertPath: cfg.WebhookServer.TLSCertPath,
            KeyPath:  cfg.WebhookServer.TLSKeyPath,
        },
        DefaultValidator: &MyGenericValidator{},
        DefaultMutator: &MyGenericMutator{},
        // For example purposes, we'll set converters and admission controllers for MyKind in the config, 
        // and add a converter and admission controllers for OtherKind via methods on the operator.
        Converters: map[metav1.GroupKind]k8s.Converter{
            metav1.GroupKind{myKind.Group(), myKind.Kind()}: &MyKindConverter{},
        },
        Validators: map[*resource.Kind]resource.ValidatingAdmissionController{
            &myKind: &MyKindValidator{},
        },
        Mutators: map[*resource.Kind]resource.MutatingAdmissionController{
            &myKind: &MyKindMutator{},
        },
    },
    Metrics: simple.MetricsConfig{ 
        Enabled: true, // Sets up a prometheus scrape endpoint at /metrics
    },
    Tracing: simple.TracingConfig{
        Enabled: true, // Sets up tracing
        OpenTelemetryConfig: simple.OpenTelemetryConfig{ // Configuration for the OTel collector
            Host:        cfg.OTelConfig.Host,
            Port:        cfg.OTelConfig.Port,
            ConnType:    OTelConnTypeGRPC,
            ServiceName: "test",
        },
    },
    ErrorHandler: func(ctx context.Context, err error) {
        logging.FromContext(ctx).Error("Something bad happened!", "error", err)
    }
})
// Use a watcher for MyKind, and a reconciler for OtherKind
// We do nothing with errors here to conserve space in this example
err = op.WatchKind(myKind, &MyKindWatcher{}, simple.ListWatchOptions{
    Namespace: resource.NamespaceAll,
    LabelFilters: []string{"foo=bar"},
})
err = op.ReconcileKind(otherKind, &OtherKindReconciler{}, simple.ListWatchOptions{
    Namespace: resource.NamespaceAll,
    LabelFilters: []string{"foo=bar"},
})
err = op.ConvertKind(metav1.GroupKind{otherKind.Group(), otherKind.Kind()}, &OtherKindConverter{})
err = op.ValidateKind(otherKind, &OtherKindValidator{})
err = op.MutateKind(otherKind, &OtherKindMutator{})
```

Once you have an operator, you can run it with the `Run` method, which will block until an error occurs that cannot be handled, or the provided channel is closed.

## Complex Operator

There are scenarios where you may need to do more than what `simple.Operator` can do for you. In those cases, you can construct an `operator.Operator` yourself, managing all added `operator.Controller` instances in your own code. Since the `operator.Controller` interface is just
```go
type Controller interface {
	Run(<-chan struct{}) error
}
```
Almost anything can be used as a controller. The SDK provides several pre-built controllers to use for most use-cases:
* `operator.InformerController` handles resource informers and tying them to watchers and/or reconcilers, along with error handling and retry logic
* `k8s.WebhookServer` handles serving the HTTPS endpoints for kubernetes `/validate`, `/mutate`, and `/convert` webhooks
* `metrics.Exporter` handles exposing a prometheus scrape endpoint (`/metrics`) for metrics

These are all used by the `simple.Operator` to some degree, and _most_ of what they accomplish can be done via the `simple.Operator`. However, some more complex things (such as the `RetryPolicy` or `DequeuePolicy` in the `operator.InformerController`) are not configurable by default in the `simple` workflow, and more advanced use-cases may want to construct an operator themselves. The `simple` workflow also introduces some opinionated logic that a user may want to avoid for their use-case. A user may also wish to add further controllers that accomplish other tasks to their operator, which can be done via the method `AddController` on the `operator.Operator` type.

To accomplish the same thing as the above "full" example of a `simple.Operator`, a non-simple one would look something like this:
```go
myKind := mykindv1.Kind()
otherKind := otherkindv1.Kind()
// Empty operator we'll add controllers to
op := operator.New()
// InformerController to hold all our informers and watchers and reconcilers
// We could also do a single InformerController per kind if we wanted to separate out things like RetryPolicy
informerController := operator.NewInformerController(operator.DefaultInformerControllerConfig())
informerController.ErrorHandler = func(ctx context.Context, err error) {
    logging.FromContext(ctx).Error("Something bad happened (in our informer controller)!", "error", err)
}
// ClientGenerator for creating Client instances requires to list/watch/patch our kinds
clientGenerator := k8s.NewClientRegistry(kubeConfig, k8s.ClientConfig{})
// Client for MyKind, we're again doing nothing with errors to conserve space
myKindClient, err := clientGenerator.ClientFor(myKind)
// Informer for MyKind, watching all namespaces with a `foo=bar` label matcher
myKindInformer, err := operator.NewKubernetesBasedInformerWithFilters(myKind, myKindClient, resource.NamespaceAll, "foo=bar")
myKindInformer.ErrorHandler = func(ctx context.Context, err error) {
    logging.FromContext(ctx).Error("Something bad happened (in the MyKind informer)!", "error", err) // We can make error handling more specific
}
// OpinionatedWatcher for MyKind, we'll use this to wrap the &MyKindWatcher{}
myKindOpinionatedWatcher, err := NewOpinionatedWatcherWithFinalizer(myKind, myKindClient, func(sch resource.Schema) string {
    return "my-operator-mykind-finalizer"
})
myKindWatcher := &MyKindWatcher{}
myKindOpinionatedWatcher.Wrap(myKindWatcher, false)
myKindOpinionatedWatcher.SyncFunc = myKindWatcher.Sync
// Add both the informer and watcher to the InformerController using the same key to associate them to each other
err = informerController.AddInformer(myKindInformer, "mykind")
err = informerController.AddWatcher(myKindOpinionatedWatcher, "mykind")
// Now we do the same thing for OtherKind
otherKindClient, err := clientGenerator.ClientFor(otherKind)
// Informer for OtherKind, watching all namespaces with a `foo=bar` label matcher
otherKindInformer, err := operator.NewKubernetesBasedInformerWithFilters(otherKind, otherKindClient, resource.NamespaceAll, "foo=bar")
otherKindInformer.ErrorHandler = func(ctx context.Context, err error) {
    logging.FromContext(ctx).Error("Something bad happened (in the OtherKind informer)!", "error", err) // We can make error handling more specific
}
// OpinionatedReconciler for OtherKind, we'll use this to wrap &OtherKindReconciler{}
otherKindOpinionatedReconciler, err := operator.NewOpinionatedReconciler(otherKindClient, "my-operator-otherkind-finalizer")
otherKindOpinionatedReconciler.Reconciler = &OtherKindReconciler{}
// Add both the informer and watcher to the InformerController using the same key to associate them to each other
err = informerController.AddInformer(otherKindInformer, "otherkind")
err = informerController.AddWatcher(otherKindOpinionatedReconciler, "otherkind")
// Add the InformerController to the operator
op.AddController(informerController)
// Set up the webhook server
webhookServer, err = k8s.NewWebhookServer(k8s.WebhookServerConfig{
    Port:      8443,
    TLSConfig: k8s.TLSConfig{
        CertPath: cfg.WebhookServer.TLSCertPath,
        KeyPath:  cfg.WebhookServer.TLSKeyPath,
    },
    DefaultValidatingController: &MyGenericValidator{},
    DefaultMutatingController:   &MyGenericMutator{},
    ValidatingControllers:       map[*resource.Kind]resource.ValidatingAdmissionController{
        &myKind:    &MyKindValidator{},
        &otherKind: &OtherKindValidator{},
    },
    MutatingControllers: map[*resource.Kind]resource.MutatingAdmissionController{
        &myKind:    &MyKindMutator{},
        &otherKind: &OtherKindMutator{},
    },
    KindConverters: map[metav1.GroupKind]k8s.Converter{
        metav1.GroupKind{myKind.Group(), myKind.Kind()}: &MyKindConverter{},
        metav1.GroupKind{otherKind.Group(), otherKind.Kind()}: &OtherKindConverter{},
    },
})
// Add the webhook server to the operator
op.AddController(webhookServer)
// Set up the metrics exporter
metricsExporter := metrics.NewExporter(metrics.DefaultConfig(""))
// Register the collectors from the other two operators and the ClientGenerator
err = metricsExporter.RegisterCollectors(informerController.PrometheusCollectors()...)
err = metricsExporter.RegisterCollectors(webhookServer.PrometheusCollectors()...)
err = metricsExporter.RegisterCollectors(clientGenerator.PrometheusCollectors()...)
// Add the metrics exporter to the operator
op.AddController(metricsExporter)
// We're going to still use the `simple` package to set up tracing, 
// Because it's not part of the operator itself
err = simple.SetTraceProvider(simple.OpenTelemetryConfig{
    Host:        cfg.OTelConfig.Host,
    Port:        cfg.OTelConfig.Port,
    ConnType:    OTelConnTypeGRPC,
    ServiceName: "test",
})
```
There's a lot more code there, and it's a bit more complicated to follow, but now you have the option to tweak many more things. We can customize error handling for each component, or mess with the configuration and settings of informers or the informer controller (we could even use different informer controllers for different kinds if we wanted to use separate retry or dequeue policies). We could remove the opinionated wrappers on the watcher or reconciler (or both). Tracing setup is decoupled from the operator entirely here, so we can set it up however we like. Not using `simple` gives us many more options, at the expense of a more complex workflow.

## Considerations When Writing an Operator

When writing an operator, it's important to take a few things into consideration:
* If you make an update to the object you're doing the reconcile (or watch) event for, this will trigger _another_ reconcile (or watch) event. Generally, favor only updating subresources (specifically `status`) and some metadata in your reconcile (or watch) events, as a `status` update should not trigger the `metadata.generation` value to increase (only `metadata.resourceVersion`), which will allow you to filter events out. Using the `operator.OpinionatedWatcher` and `operator.OpinionatedReconciler` will filter these events for you; if you prefer not to use them or want to do your own event filtering, keep in mind how updates within your reconcile loop will be received.
* the operator is taking action on _every_ consumed event. Finding ways to escape from a reconcile or watcher event early will help your overall program logic. 
* all objects for the kind(s) you are watching are cached to memory by default (there is [an open issue](https://github.com/grafana/grafana-app-sdk/issues/263) to allow customization of this).
* don't rely on retries to track operator state; use the `status` subresource to track operator success/failure, so that your operator can work out state from a fresh start (a restart will remove all pending retries, which are stored purely in-memory). This also allows a user to track operator status by viewing the `status` subresource.
* if your reconcile process makes requests for other resources, consider caching, as high-traffic objects may cause your application to have to make these requests extremely frequently.
* If your operator has a watcher or reconciler that updates the resource in a deterministic way (such as adding a label based on the spec), consider using a `MutatingAdmissionController` instead, as it makes that process synchronous and will never leave the object in an intermediate state (and reduces calls to the API server from your operator).
* When you have multiple versions of a kind, your reconciliation should only deal with one of them (typically the latest), as events are always issued for any version as the version requested by the operator's watch (so a user creating a `v1` version of a resource will still produce a `v2` version of that resource in a watch request for the `v2` of the kind).
* CRD's have a built-in conversion mechanism that is roughly equivalent to running `json.Marshal` on the stored version and then `json.Unmarshal` into the requested version. If this is not good enough for your purposes, add a version conversion webhook.