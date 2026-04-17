package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/puzpuzpuz/xsync/v2"
	"golang.org/x/sync/singleflight"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

// DynamicPatcher is a client which will always patch against the current preferred version of a kind.
// It keeps a dynamic client and discovery client per API group, so that installations routing
// different groups to different API servers (via ClientConfig.KubeConfigProvider) are handled correctly.
type DynamicPatcher struct {
	configForGroup func(group string) rest.Config

	dynamicClients   *xsync.MapOf[string, *dynamic.DynamicClient]
	discoveryClients *xsync.MapOf[string, *discovery.DiscoveryClient]
	preferred        *xsync.MapOf[string, metav1.APIResource] // keyed by schema.GroupKind.String()
	lastUpdate       *xsync.MapOf[string, time.Time]          // keyed by API group

	updateInterval time.Duration
	group          singleflight.Group
}

// NewDynamicPatcher returns a new DynamicPatcher that resolves the rest.Config for each API group
// via ClientGenerator.KubeConfigForGroup, and cacheUpdateInterval as the interval to refresh its
// preferred version cache from the API server.
// To disable the cache refresh (and only update on first request and whenever ForceRefresh() is called),
// set this value to <= 0.
func NewDynamicPatcher(clients resource.ClientGenerator, cacheUpdateInterval time.Duration) (*DynamicPatcher, error) {
	if clients == nil {
		return nil, errors.New("ClientGenerator cannot be nil")
	}
	return &DynamicPatcher{
		configForGroup:   clients.KubeConfigForGroup,
		dynamicClients:   xsync.NewMapOf[*dynamic.DynamicClient](),
		discoveryClients: xsync.NewMapOf[*discovery.DiscoveryClient](),
		preferred:        xsync.NewMapOf[metav1.APIResource](),
		lastUpdate:       xsync.NewMapOf[time.Time](),
		updateInterval:   cacheUpdateInterval,
	}, nil
}

type DynamicKindPatcher struct {
	patcher   *DynamicPatcher
	groupKind schema.GroupKind
}

func (d *DynamicKindPatcher) Get(ctx context.Context, identifier resource.Identifier) (*resource.UnstructuredWrapper, error) {
	return d.patcher.Get(ctx, d.groupKind, identifier)
}

func (d *DynamicKindPatcher) Patch(ctx context.Context, identifier resource.Identifier, patch resource.PatchRequest, options resource.PatchOptions) (resource.Object, error) {
	return d.patcher.Patch(ctx, d.groupKind, identifier, patch, options)
}

func (d *DynamicPatcher) Patch(ctx context.Context, groupKind schema.GroupKind, identifier resource.Identifier, patch resource.PatchRequest, opts resource.PatchOptions) (*resource.UnstructuredWrapper, error) {
	preferred, err := d.getPreferred(groupKind)
	if err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("patching with dynamic client", "group", groupKind.Group, "version", preferred.Version, "kind", groupKind.Kind, "plural", preferred.Name)
	data, err := marshalJSONPatch(patch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patch: %w", err)
	}
	client, err := d.getDynamicClient(groupKind.Group)
	if err != nil {
		return nil, err
	}
	res := client.Resource(schema.GroupVersionResource{
		Group:    preferred.Group,
		Version:  preferred.Version,
		Resource: preferred.Name,
	})
	subresources := make([]string, 0)
	if opts.Subresource != "" {
		subresources = append(subresources, opts.Subresource)
	}
	patchOpts := metav1.PatchOptions{}
	if opts.DryRun {
		patchOpts.DryRun = []string{"All"}
	}
	if preferred.Namespaced {
		resp, err := res.Namespace(identifier.Namespace).Patch(ctx, identifier.Name, types.JSONPatchType, data, patchOpts, subresources...)
		if err != nil {
			return nil, d.parseError(err)
		}
		return resource.NewUnstructuredWrapper(resp), nil
	}
	resp, err := res.Patch(ctx, identifier.Name, types.JSONPatchType, data, patchOpts, subresources...)
	if err != nil {
		return nil, d.parseError(err)
	}
	return resource.NewUnstructuredWrapper(resp), nil
}

func (d *DynamicPatcher) Get(ctx context.Context, groupKind schema.GroupKind, identifier resource.Identifier) (*resource.UnstructuredWrapper, error) {
	preferred, err := d.getPreferred(groupKind)
	if err != nil {
		return nil, err
	}
	logging.FromContext(ctx).Debug("getting with dynamic client", "group", groupKind.Group, "version", preferred.Version, "kind", groupKind.Kind, "plural", preferred.Name)
	client, err := d.getDynamicClient(groupKind.Group)
	if err != nil {
		return nil, err
	}
	res := client.Resource(schema.GroupVersionResource{
		Group:    preferred.Group,
		Version:  preferred.Version,
		Resource: preferred.Name,
	})
	if preferred.Namespaced {
		resp, err := res.Namespace(identifier.Namespace).Get(ctx, identifier.Name, metav1.GetOptions{})
		if err != nil {
			return nil, d.parseError(err)
		}
		return resource.NewUnstructuredWrapper(resp), nil
	}
	resp, err := res.Get(ctx, identifier.Name, metav1.GetOptions{})
	if err != nil {
		return nil, d.parseError(err)
	}
	return resource.NewUnstructuredWrapper(resp), nil
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
// for which a discovery client has already been created.
func (d *DynamicPatcher) ForceRefresh() error {
	var rangeErr error
	d.discoveryClients.Range(func(group string, _ *discovery.DiscoveryClient) bool {
		if err := d.updatePreferred(group); err != nil {
			rangeErr = err
			return false
		}
		return true
	})
	return rangeErr
}

func (d *DynamicPatcher) getPreferred(kind schema.GroupKind) (*metav1.APIResource, error) {
	_, err, _ := d.group.Do("check-cache-update:"+kind.Group, func() (any, error) {
		last, _ := d.lastUpdate.Load(kind.Group)
		if last.IsZero() || (d.updateInterval >= 0 && last.Before(now().Add(-d.updateInterval))) {
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
	disc, err := d.getDiscoveryClient(group)
	if err != nil {
		return err
	}
	preferred, err := disc.ServerPreferredResources()
	if err != nil {
		// There are errors that are "partial" errors and still return results.
		// In those cases, we should check into the error further rather than just returning.
		// If there are no results, return the error we got
		if len(preferred) == 0 {
			var statusErr *apierrors.StatusError
			if errors.As(err, &statusErr) {
				return statusErr
			}
			return fmt.Errorf("error getting preferred resources from discovery client: %w", err)
		}
		if cast, ok := err.(*discovery.ErrGroupDiscoveryFailed); ok {
			// Failed discovery for a number of groups. Log the failed groups
			for g, gerr := range cast.Groups {
				logging.DefaultLogger.Warn(fmt.Sprintf("discovery failed for GroupVersion %s", g.String()), "groupversion", g, "error", gerr)
			}
		} else {
			// just log the error
			logging.DefaultLogger.Warn("error getting preferred resources, returned partial results", "error", err)
		}
	}
	for _, pref := range preferred {
		gv, err := schema.ParseGroupVersion(pref.GroupVersion)
		if err != nil {
			return err
		}
		// Only cache entries for the group we queried. In multi-host setups, a different
		// API server may be authoritative for another group.
		if gv.Group != group {
			continue
		}
		for _, res := range pref.APIResources {
			if res.Version == "" {
				res.Version = gv.Version
			}
			if res.Group == "" {
				res.Group = gv.Group
			}
			d.preferred.Store(schema.GroupKind{Group: gv.Group, Kind: res.Kind}.String(), res)
		}
	}
	d.lastUpdate.Store(group, now())
	return nil
}

func (d *DynamicPatcher) getDynamicClient(group string) (*dynamic.DynamicClient, error) {
	if client, ok := d.dynamicClients.Load(group); ok {
		return client, nil
	}
	cfg := d.configForGroup(group)
	client, err := dynamic.NewForConfig(&cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating dynamic client for group %q: %w", group, err)
	}
	actual, _ := d.dynamicClients.LoadOrStore(group, client)
	return actual, nil
}

func (d *DynamicPatcher) getDiscoveryClient(group string) (*discovery.DiscoveryClient, error) {
	if disc, ok := d.discoveryClients.Load(group); ok {
		return disc, nil
	}
	cfg := d.configForGroup(group)
	disc, err := discovery.NewDiscoveryClientForConfig(&cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating discovery client for group %q: %w", group, err)
	}
	actual, _ := d.discoveryClients.LoadOrStore(group, disc)
	return actual, nil
}

func (*DynamicPatcher) parseError(err error) error {
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return NewServerResponseError(statusErr, statusErr.Status().Code)
	}
	return err
}
