package k8s

import (
	"errors"
	"fmt"

	"github.com/puzpuzpuz/xsync/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

var _ resource.DiscoveryClient = (*DiscoveryClient)(nil)

// DiscoveryClient is a resource.DiscoveryClient backed by one or more client-go discovery clients.
// In installations where different API groups are routed to different API servers
// (via ClientConfig.KubeConfigProvider / NewClientConfigWithExternalClients), it keeps a separate
// discovery client per group so each group hits its authoritative API server.
type DiscoveryClient struct {
	defaultKubeConfig  rest.Config
	kubeConfigProvider func(kind resource.Kind, kubeConfig rest.Config) rest.Config
	clients            *xsync.MapOf[string, *discovery.DiscoveryClient]
}

// NewDiscoveryClient returns a new DiscoveryClient. If kubeConfigProvider is non-nil, it is used
// to resolve the rest.Config for each API group (called with a synthetic Kind whose only
// populated field is Group, matching the pattern used by DefaultClientConfig).
func NewDiscoveryClient(cfg rest.Config, kubeConfigProvider func(kind resource.Kind, kubeConfig rest.Config) rest.Config) *DiscoveryClient {
	return &DiscoveryClient{
		defaultKubeConfig:  cfg,
		kubeConfigProvider: kubeConfigProvider,
		clients:            xsync.NewMapOf[*discovery.DiscoveryClient](),
	}
}

// APIGroupInfo returns the preferred-version resources exposed by the given API group.
// Each entry's Group and Version fields are populated with the resource's preferred GroupVersion.
func (d *DiscoveryClient) APIGroupInfo(apiGroup string) ([]metav1.APIResource, error) {
	client, err := d.getClient(apiGroup)
	if err != nil {
		return nil, err
	}
	preferred, err := client.ServerPreferredResources()
	var targetGroupErr error
	if err != nil {
		// There are errors that are "partial" errors and still return results.
		// In those cases, we should check into the error further rather than just returning.
		// If there are no results, return the error we got.
		if len(preferred) == 0 {
			var statusErr *apierrors.StatusError
			if errors.As(err, &statusErr) {
				return nil, statusErr
			}
			return nil, fmt.Errorf("error getting preferred resources from discovery client: %w", err)
		}
		var groupDiscoveryErr *discovery.ErrGroupDiscoveryFailed
		if errors.As(err, &groupDiscoveryErr) {
			for g, gerr := range groupDiscoveryErr.Groups {
				logging.DefaultLogger.Warn(fmt.Sprintf("discovery failed for GroupVersion %s", g.String()), "groupversion", g, "error", gerr)
				if g.Group == apiGroup {
					targetGroupErr = fmt.Errorf("discovery failed for group %q: %w", apiGroup, gerr)
				}
			}
		} else {
			logging.DefaultLogger.Warn("error getting preferred resources, returned partial results", "error", err)
		}
	}
	var resources []metav1.APIResource
	for _, pref := range preferred {
		gv, err := schema.ParseGroupVersion(pref.GroupVersion)
		if err != nil {
			return nil, err
		}
		// In multi-host setups, a different API server may be authoritative for another group,
		// so only include entries for the group we queried.
		if gv.Group != apiGroup {
			continue
		}
		for _, res := range pref.APIResources {
			if res.Version == "" {
				res.Version = gv.Version
			}
			if res.Group == "" {
				res.Group = gv.Group
			}
			resources = append(resources, res)
		}
	}

	return resources, targetGroupErr
}

func (d *DiscoveryClient) kubeConfigForGroup(group string) rest.Config {
	cfg := d.defaultKubeConfig
	if d.kubeConfigProvider != nil {
		sch := resource.NewSimpleSchema(group, "", &resource.UntypedObject{}, &resource.UntypedList{})
		cfg = d.kubeConfigProvider(resource.Kind{Schema: sch}, cfg)
	}
	return cfg
}

func (d *DiscoveryClient) getClient(group string) (*discovery.DiscoveryClient, error) {
	if c, ok := d.clients.Load(group); ok {
		return c, nil
	}
	// Compute holds the shard lock for `group`, so concurrent first-time callers for the same
	// group won't both construct a discovery client. On failure we return delete=true so the
	// failure isn't cached and a subsequent call gets to retry.
	var createErr error
	c, _ := d.clients.Compute(group, func(existing *discovery.DiscoveryClient, loaded bool) (*discovery.DiscoveryClient, bool) {
		if loaded {
			return existing, false
		}
		cfg := d.kubeConfigForGroup(group)
		newClient, err := discovery.NewDiscoveryClientForConfig(&cfg)
		if err != nil {
			createErr = err
			return nil, true
		}
		return newClient, false
	})
	if createErr != nil {
		return nil, fmt.Errorf("error creating discovery client for group %q: %w", group, createErr)
	}
	return c, nil
}
