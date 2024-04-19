package apiserver

import (
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

// ReconcilersPostStartHook returns a PostStartHook function that creates a controller and adds an informer/reconciler
// pair for each Reconciler in a Resource in the ResourceGroup(s). The informer(s) will use a client from the
// PostStartHookContext, and the controller will run indefinitely until the PostStartContext's StopCh is closed.
func ReconcilersPostStartHook(getter OptionsGetter, groups ...ResourceGroup) func(ctx genericapiserver.PostStartHookContext) error {
	return func(ctx genericapiserver.PostStartHookContext) error {
		// We need the loopback config to run reconcilers
		if ctx.LoopbackClientConfig == nil {
			return fmt.Errorf("missing LoopbackClientConfig from PostStartHookContext")
		}

		// We have to fix some aspects of the loopback config, like adding /apis, and replacing [::1] if the host is set to that,
		// otherwise the kubernetes client can't talk to the API server over the interface
		ctx.LoopbackClientConfig.Host = strings.Replace(ctx.LoopbackClientConfig.Host, "[::1]", "127.0.0.1", 1)
		ctx.LoopbackClientConfig.APIPath = "/apis"

		// Create the client registry from the loopback config, and controller we'll be running our reconcilers and informers in
		clientRegistry := k8s.NewClientRegistry(*ctx.LoopbackClientConfig, k8s.DefaultClientConfig())
		controller := operator.NewInformerController(operator.DefaultInformerControllerConfig())

		// Keep a count of the reconcilers we add to the controller, if we don't add any we don't have to run the controller
		reconcilerCount := 0
		for _, g := range groups {
			for _, r := range g.Resources {
				if r.Reconciler != nil {
					kindStr := fmt.Sprintf("%s.%s/%s", r.Kind.Plural(), r.Kind.Group(), r.Kind.Version())
					reconciler, err := r.Reconciler(clientRegistry, getter)
					if err != nil {
						return fmt.Errorf("error creating reconciler for %s: %w", kindStr, err)
					}
					err = controller.AddReconciler(reconciler, kindStr)
					if err != nil {
						return fmt.Errorf("error adding reconciler for %s: %w", kindStr, err)
					}
					client, err := clientRegistry.ClientFor(r.Kind)
					if err != nil {
						return fmt.Errorf("error creating kubernetes client for %s: %w", kindStr, err)
					}
					informer, err := operator.NewKubernetesBasedInformer(r.Kind, client, resource.NamespaceAll)
					if err != nil {
						return fmt.Errorf("error creating informer for %s: %w", kindStr, err)
					}
					err = controller.AddInformer(informer, kindStr)
					if err != nil {
						return fmt.Errorf("error adding informer for %s to controller: %w", kindStr, err)
					}
					reconcilerCount++
				}
			}
		}
		// Run the controller in a goroutine. It will run until ctx.StopCh is closed
		if reconcilerCount > 0 {
			go controller.Run(ctx.StopCh)
		}
		return nil
	}
}
