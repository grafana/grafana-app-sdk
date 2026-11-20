package simple

import (
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerMechanism is a way of watching a kind which says what it gives that kind's handlers, so that several can
// be selected between. Implement it on a supplier which knows its own configuration — an event bus knows its payload
// and delivery group — so what it advertises cannot drift from what it builds. It answers per kind, since what it
// can provide may depend on the kind.
//
// Experimental: this API may change in a minor release.
type InformerMechanism interface {
	// WatchOptions is what the informers this mechanism builds give a kind's handlers.
	WatchOptions(kind resource.Kind) operator.WatchOptions
	// NewInformer builds the informer for a kind, as an InformerSupplier does.
	NewInformer(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error)
}

// NewInformerMechanism presents a supplier whose provision is fixed as an InformerMechanism.
//
// Experimental: this API may change in a minor release.
func NewInformerMechanism(provides operator.WatchOptions, supplier InformerSupplier) InformerMechanism {
	return fixedMechanism{provides: provides, supplier: supplier}
}

type fixedMechanism struct {
	provides operator.WatchOptions
	supplier InformerSupplier
}

func (f fixedMechanism) WatchOptions(resource.Kind) operator.WatchOptions {
	return f.provides
}

func (f fixedMechanism) NewInformer(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.Informer, error) {
	return f.supplier(kind, clients, options)
}

// NewSelectingInformerSupplier returns an InformerSupplier which gives each kind the first of these mechanisms
// which satisfies what its handlers need, asking each what it provides for that kind. A nil mechanism is skipped,
// which is how one whose transport is switched off is listed. A kind none satisfies is built by fallback, which is
// DefaultInformerSupplier when nil — and the fallback is checked like any other mechanism, so a declaration the API
// watch cannot serve either, such as a divided channel, fails at construction rather than being served by less.
//
// The list is the whole policy: each kind gets the most optimal mechanism available to it, and the order says which
// that is. A deployment with all three usually lists the event bus, then the re-list, then the watch.
//
// It is an InformerSupplier like any other, so it goes in AppInformerConfig.InformerSupplier and reads the kind's
// needs off InformerOptions.WatchNeeds. Wrap it to answer for one kind and defer to it for the rest.
//
// Experimental: this API may change in a minor release.
func NewSelectingInformerSupplier(fallback InformerSupplier, mechanisms ...InformerMechanism) InformerSupplier {
	if fallback == nil {
		fallback = DefaultInformerSupplier
	}
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		for _, mechanism := range mechanisms {
			if mechanism == nil || !options.WatchNeeds.SatisfiedBy(mechanism.WatchOptions(kind)) {
				continue
			}
			return mechanism.NewInformer(kind, clients, options)
		}
		return fallback(kind, clients, options)
	}
}

// informerFor builds the informer for a kind the App manages or watches.
func (a *App) informerFor(kind AppUnmanagedKind) (operator.Informer, error) {
	opts := a.cfg.InformerConfig.InformerOptions
	if opts.MetricsConfig == (metrics.Config{}) {
		opts.MetricsConfig = a.cfg.InformerConfig.MetricsConfig
	}
	return buildInformer(informerRequest{
		kind:     kind.Kind,
		declared: kind.ReconcileOptions.WatchOptions,
		listWatch: operator.ListWatchOptions{
			Namespace:      kind.ReconcileOptions.Namespace,
			LabelFilters:   kind.ReconcileOptions.LabelFilters,
			FieldSelectors: kind.ReconcileOptions.FieldSelectors,
		},
		options:     opts,
		supplier:    a.cfg.InformerConfig.InformerSupplier,
		clients:     a.clientGenerator,
		opinionated: kind.ReconcileOptions.UseOpinionated,
	})
}

// informerRequest is what building one kind's informer takes, shared by the App and the Operator.
type informerRequest struct {
	kind resource.Kind
	// declared is what the kind stated, before defaults.
	declared    operator.WatchOptions
	listWatch   operator.ListWatchOptions
	options     operator.InformerOptions
	supplier    InformerSupplier
	clients     resource.ClientGenerator
	opinionated bool
}

// buildInformer asks the supplier for this kind's informer, hands it the kind's needs, and checks what came back.
func buildInformer(req informerRequest) (operator.Informer, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}

	supplier := req.supplier
	if supplier == nil {
		supplier = DefaultInformerSupplier
	}

	opts := req.options
	opts.ListWatchOptions = req.listWatch
	// The mechanism resolves its cadence and page size from these with operator.InformerOptions.ResyncOptions,
	// so nothing here rewrites the options it was configured with.
	opts.WatchNeeds = req.watchNeeds()

	inf, err := supplier(req.kind, req.clients, opts)
	if err != nil {
		return nil, err
	}
	if err := req.checkProvision(opts, inf); err != nil {
		return nil, err
	}
	return inf, nil
}

// watchNeeds is what this kind's handlers need, with defaults filled in. The Opinionated variants read the deletion
// timestamp and the finalizer list off the object on the event, so they need whole objects on every channel. The
// declaration is copied rather than defaulted in place: it belongs to the caller's config.
func (r informerRequest) watchNeeds() operator.WatchOptions {
	needs := operator.WatchOptions{}
	if r.declared.Live == nil && r.declared.Resync == nil {
		needs.Live = &operator.WatchLiveOptions{}
	} else {
		if r.declared.Live != nil {
			live := *r.declared.Live
			needs.Live = &live
		}
		if r.declared.Resync != nil {
			resync := *r.declared.Resync
			needs.Resync = &resync
		}
	}
	if needs.Live != nil {
		if needs.Live.Detail == "" {
			needs.Live.Detail = operator.WatchDetailFullObject
		}
		if needs.Live.Distribution == "" {
			needs.Live.Distribution = operator.WatchDistributionAll
		}
	}
	if needs.Resync != nil {
		if needs.Resync.Detail == "" {
			needs.Resync.Detail = operator.WatchDetailFullObject
		}
		if needs.Resync.Distribution == "" {
			needs.Resync.Distribution = operator.WatchDistributionAll
		}
	}
	return needs
}

// validate rejects a declaration its handler cannot be served by whatever mechanism satisfies it. The Opinionated
// variants read the deletion timestamp and the finalizer list off the object on the event, so identities are a
// contradiction rather than something to quietly upgrade.
func (r informerRequest) validate() error {
	if !r.opinionated {
		return nil
	}
	if channels := identityChannels(r.declared); len(channels) > 0 {
		return fmt.Errorf("kind '%s' declares %s identities, which its opinionated reconciler or watcher cannot be"+
			" served by: it reads the deletion timestamp and the finalizer list off the object on the event. Declare"+
			" %q, or set UsePlain (UseOpinionated false for an unmanaged kind) and handle deletion in your own"+
			" reconcile",
			r.kind.GroupVersionKind().String(), strings.Join(channels, " and "), operator.WatchDetailFullObject)
	}
	return nil
}

// identityChannels names the channels of o which carry no more than an identity.
func identityChannels(o operator.WatchOptions) []string {
	var channels []string
	if o.Live != nil && o.Live.Detail == operator.WatchDetailIdentity {
		channels = append(channels, "Live")
	}
	if o.Resync != nil && o.Resync.Detail == operator.WatchDetailIdentity {
		channels = append(channels, "Resync")
	}
	return channels
}

// checkProvision reports an error when an informer which says what it provides provides less than its kind's
// handlers need. One which says nothing is not checked: nothing can be read off it.
func (r informerRequest) checkProvision(opts operator.InformerOptions, inf operator.Informer) error {
	needs := opts.WatchNeeds
	provided, stated := operator.InformerWatchOptions(inf)
	if !stated {
		return nil
	}
	// Every channel the mechanism delivers on reaches an opinionated wrapper, including one this kind did not ask
	// for and so did not have compared, and identities on any of them break its deletion handling.
	if channels := identityChannels(provided); r.opinionated && len(channels) > 0 {
		return fmt.Errorf("informer for kind '%s' carries identities on its %s channel, which reaches its opinionated"+
			" reconciler or watcher whether the kind asked for that channel or not and leaves it unable to tell a"+
			" delete in progress from a fresh object; set UsePlain (UseOpinionated false for an unmanaged kind), or"+
			" give this kind a mechanism carrying whole objects on every channel",
			r.kind.GroupVersionKind().String(), strings.Join(channels, " and "))
	}
	if needs.SatisfiedBy(provided) {
		return nil
	}
	handler := "its reconciler or watcher"
	if r.opinionated {
		handler = "its opinionated reconciler or watcher"
	}
	remedy := ""
	// The one need nothing anywhere was configured for: a resync with no cadence to run it at.
	resolved := opts.ResyncOptions()
	if needs.Resync != nil && provided.Resync == nil && (resolved == nil || resolved.Interval == 0) {
		remedy = "; set WatchOptions.Resync.Interval for the kind, or " +
			"InformerConfig.InformerOptions.CacheResyncInterval for the app, so that a mechanism has a cadence to resync at"
	}
	return fmt.Errorf("informer for kind '%s' provides %s, but %s needs %s%s",
		r.kind.GroupVersionKind().String(), provided, handler, needs, remedy)
}
