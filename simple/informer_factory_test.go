package simple

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

func TestWatchDeclarationsFromManifest(t *testing.T) {
	data := app.ManifestData{Group: "g", Versions: []app.ManifestVersion{
		{Name: "v1", Kinds: []app.ManifestVersionKind{
			{Kind: "Widget", Watch: &app.ManifestVersionKindWatch{
				Needs: app.WatchNeedsSignal,
				Metadata: &app.ManifestVersionKindWatchMetadata{
					Labels:     []string{"app"},
					Finalizers: []string{"g/finalizer"},
					Generation: true,
				},
			}},
			{Kind: "Gadget"},
		}},
		{Name: "v2", Kinds: []app.ManifestVersionKind{
			{Kind: "Widget", Watch: &app.ManifestVersionKindWatch{
				Needs: app.WatchNeedsCadence,
				Metadata: &app.ManifestVersionKindWatchMetadata{
					Labels:            []string{"tier"},
					DeletionTimestamp: true,
				},
			}},
		}},
	}}

	declarations := WatchDeclarationsFromManifest(data)

	// A kind served in several versions takes the strongest need and the union of what the versions announce: the
	// same publisher announces every version of it.
	widget := declarations["Widget"]
	assert.Equal(t, app.WatchNeedsSignal, widget.Needs, "signal is stronger than cadence")
	assert.Equal(t, []string{"app", "tier"}, widget.AnnouncedMetadata.Labels)
	assert.True(t, widget.AnnouncedMetadata.Finalizers)
	assert.True(t, widget.AnnouncedMetadata.Generation)
	assert.True(t, widget.AnnouncedMetadata.DeletionTimestamp)

	assert.Equal(t, KindWatchDeclaration{}, declarations["Gadget"], "a kind which declares nothing needs objects")
}

func TestInformerFactory_Informer(t *testing.T) {
	build := func(t *testing.T, decl KindWatchDeclaration, cfg ManifestInformerConfig) operator.Informer {
		t.Helper()
		inf, err := NewInformerFactory(cfg).Informer(testKind(), decl, stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)
		return inf
	}

	t.Run("a kind which declares nothing is served by the watch", func(t *testing.T) {
		inf := build(t, KindWatchDeclaration{}, ManifestInformerConfig{Subscriber: resourcebus.NewFake()})
		assert.False(t, inf.(*operator.ConcurrentInformer).IdentityPayload())
	})

	t.Run("needing a signal resolves to the bus when one is configured", func(t *testing.T) {
		inf := build(t, KindWatchDeclaration{Needs: app.WatchNeedsSignal},
			ManifestInformerConfig{Subscriber: resourcebus.NewFake()})
		assert.True(t, inf.(*operator.ConcurrentInformer).IdentityPayload())
	})

	t.Run("needing a signal falls back to the watch without a bus", func(t *testing.T) {
		inf := build(t, KindWatchDeclaration{Needs: app.WatchNeedsSignal}, ManifestInformerConfig{})
		assert.False(t, inf.(*operator.ConcurrentInformer).IdentityPayload())
	})

	// A bus supplier is built per kind so that each gets its own announced metadata, which is what a label filter
	// on an identity payload is evaluated against and what lets it serve an opinionated reconciler.
	t.Run("the kind's own announced metadata reaches its informer", func(t *testing.T) {
		factory := NewInformerFactory(ManifestInformerConfig{Subscriber: resourcebus.NewFake()})
		opinionated, err := factory.Informer(testKind(), KindWatchDeclaration{
			Needs: app.WatchNeedsSignal,
			AnnouncedMetadata: operator.AnnouncedMetadata{
				Finalizers: true, Generation: true, DeletionTimestamp: true,
			},
		}, stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)
		bare, err := factory.Informer(testUnmanagedKind(), KindWatchDeclaration{Needs: app.WatchNeedsSignal},
			stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)

		assert.True(t, opinionated.(*operator.ConcurrentInformer).DeliversOpinionatedMetadata())
		assert.False(t, bare.(*operator.ConcurrentInformer).DeliversOpinionatedMetadata(),
			"one kind's declaration does not leak into another's informer")
	})

	t.Run("a configured announced metadata overrides the declaration", func(t *testing.T) {
		inf := build(t, KindWatchDeclaration{Needs: app.WatchNeedsSignal}, ManifestInformerConfig{
			Subscriber: resourcebus.NewFake(),
			Bus: BusInformerConfig{AnnouncedMetadata: operator.AnnouncedMetadata{
				Finalizers: true, Generation: true, DeletionTimestamp: true,
			}},
		})
		assert.True(t, inf.(*operator.ConcurrentInformer).DeliversOpinionatedMetadata())
	})

	t.Run("every informer the factory builds shares one set of delivery collectors", func(t *testing.T) {
		var watchOptions operator.InformerOptions
		factory := NewInformerFactory(ManifestInformerConfig{
			Subscriber: resourcebus.NewFake(),
			Watch: func(_ resource.Kind, _ resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
				watchOptions = opts
				return identityInformer{}, nil
			},
		})
		_, err := factory.Informer(testKind(), KindWatchDeclaration{}, stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)
		bus, err := factory.Informer(testKind(), KindWatchDeclaration{Needs: app.WatchNeedsSignal},
			stubClients{}, operator.InformerOptions{})
		require.NoError(t, err)

		require.NotNil(t, watchOptions.DeliveryMetrics)
		assert.Subset(t, bus.(*operator.ConcurrentInformer).PrometheusCollectors(),
			watchOptions.DeliveryMetrics.PrometheusCollectors())
	})
}
