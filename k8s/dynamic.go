package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/puzpuzpuz/xsync/v2"
	"golang.org/x/sync/singleflight"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

// DynamicPatcher is a client which will always patch against the current preferred version of a kind.
// It obtains per-kind clients through the provided ClientGenerator, so installations routing
// different groups to different API servers (via ClientConfig.KubeConfigProvider) are handled correctly.
type DynamicPatcher struct {
	clients   resource.ClientGenerator
	discovery resource.DiscoveryClient

	preferred  *xsync.MapOf[string, metav1.APIResource] // keyed by schema.GroupKind.String()
	lastUpdate *xsync.MapOf[string, time.Time]          // keyed by API group

	updateInterval time.Duration
	group          singleflight.Group
}

// NewDynamicPatcher returns a new DynamicPatcher. The ClientGenerator is used to build per-kind
// clients at the preferred version (via ClientGenerator.ClientFor), and the DiscoveryClient is
// used to look up the preferred version for each API group. Callers routing different groups to
// different API servers should provide a DiscoveryClient that mirrors that routing (e.g. via
// NewDiscoveryClient with a matching KubeConfigProvider).
// cacheUpdateInterval is the interval to refresh the preferred version cache from the API server.
// To disable the cache refresh (and only update on first request and whenever ForceRefresh() is called),
// set this value to <= 0.
func NewDynamicPatcher(clients resource.ClientGenerator, discovery resource.DiscoveryClient, cacheUpdateInterval time.Duration) (*DynamicPatcher, error) {
	if clients == nil {
		return nil, errors.New("ClientGenerator cannot be nil")
	}
	if discovery == nil {
		return nil, errors.New("DiscoveryClient cannot be nil")
	}
	return &DynamicPatcher{
		clients:        clients,
		discovery:      discovery,
		preferred:      xsync.NewMapOf[metav1.APIResource](),
		lastUpdate:     xsync.NewMapOf[time.Time](),
		updateInterval: cacheUpdateInterval,
	}, nil
}

type DynamicKindPatcher struct {
	patcher   *DynamicPatcher
	groupKind schema.GroupKind
}

func (d *DynamicKindPatcher) Get(ctx context.Context, identifier resource.Identifier) (resource.Object, error) {
	return d.patcher.Get(ctx, d.groupKind, identifier)
}

func (d *DynamicKindPatcher) Patch(ctx context.Context, identifier resource.Identifier, patch resource.PatchRequest, options resource.PatchOptions) (resource.Object, error) {
	return d.patcher.Patch(ctx, d.groupKind, identifier, patch, options)
}

func (d *DynamicPatcher) Patch(ctx context.Context, groupKind schema.GroupKind, identifier resource.Identifier, patch resource.PatchRequest, opts resource.PatchOptions) (resource.Object, error) {
	client, preferred, err := d.clientForPreferred(groupKind)
	if err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("patching with preferred-version client", "group", groupKind.Group, "version", preferred.Version, "kind", groupKind.Kind, "plural", preferred.Name)
	return client.Patch(ctx, identifier, patch, opts)
}

func (d *DynamicPatcher) Get(ctx context.Context, groupKind schema.GroupKind, identifier resource.Identifier) (resource.Object, error) {
	client, preferred, err := d.clientForPreferred(groupKind)
	if err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("getting with preferred-version client", "group", groupKind.Group, "version", preferred.Version, "kind", groupKind.Kind, "plural", preferred.Name)
	return client.Get(ctx, identifier)
}

// ForKind returns a DynamicKindPatcher for the provided group and kind, which implements the Patch method from resource.Client.
// It wraps DynamicPatcher's Patch method, and will use the same self-updating cache of the preferred version
func (d *DynamicPatcher) ForKind(groupKind schema.GroupKind) *DynamicKindPatcher {
	return &DynamicKindPatcher{
		patcher:   d,
		groupKind: groupKind,
	}
}

// ForceRefresh forces an update of the preferred version cache for every API group
// that has already been queried.
func (d *DynamicPatcher) ForceRefresh() error {
	var rangeErr error
	d.lastUpdate.Range(func(group string, _ time.Time) bool {
		if err := d.updatePreferred(group); err != nil {
			rangeErr = err
			return false
		}
		return true
	})
	return rangeErr
}

func (d *DynamicPatcher) clientForPreferred(groupKind schema.GroupKind) (resource.Client, *metav1.APIResource, error) {
	preferred, err := d.getPreferred(groupKind)
	if err != nil {
		return nil, nil, err
	}
	scope := resource.NamespacedScope
	if !preferred.Namespaced {
		scope = resource.ClusterScope
	}
	sch := resource.NewSimpleSchema(groupKind.Group, preferred.Version, &resource.UntypedObject{}, &resource.UntypedList{},
		resource.WithKind(groupKind.Kind),
		resource.WithPlural(preferred.Name),
		resource.WithScope(scope),
	)
	kind := resource.Kind{
		Schema: sch,
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	client, err := d.clients.ClientFor(kind)
	if err != nil {
		return nil, nil, err
	}
	return client, preferred, nil
}

func (d *DynamicPatcher) getPreferred(kind schema.GroupKind) (*metav1.APIResource, error) {
	_, err, _ := d.group.Do("check-cache-update:"+kind.Group, func() (any, error) {
		last, _ := d.lastUpdate.Load(kind.Group)
		if last.IsZero() || (d.updateInterval > 0 && last.Before(now().Add(-d.updateInterval))) {
			if err := d.updatePreferred(kind.Group); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	pref, ok := d.preferred.Load(kind.String())
	if !ok {
		return nil, fmt.Errorf("preferred resource not found for kind '%s'", kind)
	}
	return &pref, nil
}

func (d *DynamicPatcher) updatePreferred(group string) error {
	resources, err := d.discovery.APIGroupInfo(group)
	if err != nil {
		return err
	}
	for _, res := range resources {
		d.preferred.Store(schema.GroupKind{Group: group, Kind: res.Kind}.String(), res)
	}
	d.lastUpdate.Store(group, now())
	return nil
}
