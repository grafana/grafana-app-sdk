package k8s

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// discoveryTestServer is a stub apiserver that knows a single (group, version) and records hit paths.
// Any path it doesn't explicitly handle returns a 200 with empty JSON so the discovery client's
// partial-failure path doesn't blow up during legacy `/api` probing.
type discoveryTestServer struct {
	group   string
	version string
	kind    string
	plural  string

	mu   sync.Mutex
	hits map[string]int

	srv *httptest.Server
}

func newDiscoveryTestServer(t *testing.T, group, version, kind, plural string) *discoveryTestServer {
	t.Helper()
	ds := &discoveryTestServer{
		group:   group,
		version: version,
		kind:    kind,
		plural:  plural,
		hits:    map[string]int{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ds.mu.Lock()
		ds.hits[r.URL.Path]++
		ds.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis":
			_ = json.NewEncoder(w).Encode(metav1.APIGroupList{
				TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"},
				Groups: []metav1.APIGroup{{
					Name: ds.group,
					PreferredVersion: metav1.GroupVersionForDiscovery{
						GroupVersion: ds.group + "/" + ds.version,
						Version:      ds.version,
					},
					Versions: []metav1.GroupVersionForDiscovery{{
						GroupVersion: ds.group + "/" + ds.version,
						Version:      ds.version,
					}},
				}},
			})
		case "/apis/" + ds.group + "/" + ds.version:
			_ = json.NewEncoder(w).Encode(metav1.APIResourceList{
				TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
				GroupVersion: ds.group + "/" + ds.version,
				APIResources: []metav1.APIResource{{
					Name:       ds.plural,
					Kind:       ds.kind,
					Namespaced: true,
					Verbs:      metav1.Verbs{"get", "list", "patch"},
				}},
			})
		default:
			// 200 with empty doc to keep discovery's legacy probes quiet.
			_, _ = w.Write([]byte(`{}`))
		}
	})

	ds.srv = httptest.NewServer(mux)
	t.Cleanup(ds.srv.Close)
	return ds
}

func (d *discoveryTestServer) hitCount(path string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hits[path]
}

// TestDynamicPatcher_PerGroupDiscoveryRouting proves that when a ClientRegistry is configured with
// per-group external clients (NewClientConfigWithExternalClients), DynamicPatcher's discovery lookups
// for each group land on the correct apiserver — i.e. the ClientGenerator's KubeConfigProvider is
// threaded all the way through to the DiscoveryClient.
func TestDynamicPatcher_PerGroupDiscoveryRouting(t *testing.T) {
	const (
		groupA, versionA, kindA, pluralA = "alpha.example.com", "v1", "Widget", "widgets"
		groupB, versionB, kindB, pluralB = "beta.example.com", "v2", "Gadget", "gadgets"
	)

	srvA := newDiscoveryTestServer(t, groupA, versionA, kindA, pluralA)
	srvB := newDiscoveryTestServer(t, groupB, versionB, kindB, pluralB)

	cfg := NewClientConfigWithExternalClients(map[string]*RemoteRestConfig{
		groupA: {Host: srvA.srv.URL},
		groupB: {Host: srvB.srv.URL},
	})
	// Base kubeconfig points at a host that MUST NOT be reached; if routing is broken we'll see
	// the connection failure rather than silently hitting one of the servers.
	registry := NewClientRegistry(rest.Config{Host: "http://should-not-be-called.invalid"}, cfg)

	patcher, err := NewDynamicPatcher(registry, 0)
	require.NoError(t, err)

	// Drive a discovery refresh for each group through the real code path.
	require.NoError(t, patcher.updatePreferred(groupA))
	require.NoError(t, patcher.updatePreferred(groupB))

	// Server A saw groupA's discovery probe, not groupB's.
	require.Positive(t, srvA.hitCount("/apis/"+groupA+"/"+versionA), "srvA should have served groupA discovery")
	require.Zero(t, srvA.hitCount("/apis/"+groupB+"/"+versionB), "srvA must not serve groupB discovery")

	// And vice versa.
	require.Positive(t, srvB.hitCount("/apis/"+groupB+"/"+versionB), "srvB should have served groupB discovery")
	require.Zero(t, srvB.hitCount("/apis/"+groupA+"/"+versionA), "srvB must not serve groupA discovery")

	// Cache ended up with the correct preferred version per group.
	prefA, err := patcher.getPreferred(schema.GroupKind{Group: groupA, Kind: kindA})
	require.NoError(t, err)
	require.Equal(t, versionA, prefA.Version)
	require.Equal(t, pluralA, prefA.Name)

	prefB, err := patcher.getPreferred(schema.GroupKind{Group: groupB, Kind: kindB})
	require.NoError(t, err)
	require.Equal(t, versionB, prefB.Version)
	require.Equal(t, pluralB, prefB.Name)
}

// TestDiscoveryClient_APIGroupInfo_FiltersToRequestedGroup exercises APIGroupInfo directly
// against a server that advertises multiple groups. We only asked about one, so the
// returned slice should only contain entries for that group.
func TestDiscoveryClient_APIGroupInfo_FiltersToRequestedGroup(t *testing.T) {
	const (
		wantGroup, wantVersion = "alpha.example.com", "v1"
		otherGroup             = "beta.example.com"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis":
			_ = json.NewEncoder(w).Encode(metav1.APIGroupList{
				TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"},
				Groups: []metav1.APIGroup{
					{
						Name:             wantGroup,
						PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: wantGroup + "/" + wantVersion, Version: wantVersion},
						Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: wantGroup + "/" + wantVersion, Version: wantVersion}},
					},
					{
						Name:             otherGroup,
						PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: otherGroup + "/v1", Version: "v1"},
						Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: otherGroup + "/v1", Version: "v1"}},
					},
				},
			})
		case "/apis/" + wantGroup + "/" + wantVersion:
			_ = json.NewEncoder(w).Encode(metav1.APIResourceList{
				TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
				GroupVersion: wantGroup + "/" + wantVersion,
				APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true, Verbs: metav1.Verbs{"get"}}},
			})
		case "/apis/" + otherGroup + "/v1":
			_ = json.NewEncoder(w).Encode(metav1.APIResourceList{
				TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
				GroupVersion: otherGroup + "/v1",
				APIResources: []metav1.APIResource{{Name: "gadgets", Kind: "Gadget", Namespaced: true, Verbs: metav1.Verbs{"get"}}},
			})
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(server.Close)

	dc := NewDiscoveryClient(rest.Config{Host: server.URL}, nil)
	resources, err := dc.APIGroupInfo(wantGroup)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	require.Equal(t, "Widget", resources[0].Kind)
	require.Equal(t, wantGroup, resources[0].Group)
	require.Equal(t, wantVersion, resources[0].Version)
}

// TestDynamicPatcher_ForceRefresh verifies that ForceRefresh only refreshes groups that
// have already been queried, and re-hits the apiserver for each of them.
func TestDynamicPatcher_ForceRefresh(t *testing.T) {
	const group, version, kind, plural = "alpha.example.com", "v1", "Widget", "widgets"
	srv := newDiscoveryTestServer(t, group, version, kind, plural)

	cfg := NewClientConfigWithExternalClients(map[string]*RemoteRestConfig{
		group: {Host: srv.srv.URL},
	})
	registry := NewClientRegistry(rest.Config{Host: "http://should-not-be-called.invalid"}, cfg)

	patcher, err := NewDynamicPatcher(registry, 0)
	require.NoError(t, err)

	// No groups queried yet — ForceRefresh must be a no-op (documented behavior).
	require.NoError(t, patcher.ForceRefresh())
	require.Zero(t, srv.hitCount("/apis/"+group+"/"+version))

	// Prime the cache for this group.
	_, err = patcher.getPreferred(schema.GroupKind{Group: group, Kind: kind})
	require.NoError(t, err)
	firstHits := srv.hitCount("/apis/" + group + "/" + version)
	require.Positive(t, firstHits)

	// Now ForceRefresh should re-hit the apiserver for this group.
	require.NoError(t, patcher.ForceRefresh())
	require.Greater(t, srv.hitCount("/apis/"+group+"/"+version), firstHits, "ForceRefresh should re-hit discovery")
}
