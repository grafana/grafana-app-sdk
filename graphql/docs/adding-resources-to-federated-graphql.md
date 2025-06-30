# Adding Resources to GraphQL Federation

This guide provides a step-by-step recipe for extending Grafana's GraphQL federation system to include new App SDK-backed resources that are registered in the Grafana apps registry.

## Overview

The GraphQL federation system automatically discovers and integrates App SDK-backed resources by:

1. **Auto-discovery**: Scanning app providers that implement `GraphQLSubgraphProvider`
2. **Schema generation**: Converting CUE kinds to GraphQL types and resolvers
3. **Storage bridging**: Reusing existing REST storage without data migration
4. **Field prefixing**: Namespacing fields to avoid conflicts (e.g., `playlist_playlist`)

## Prerequisites

Before adding GraphQL support to your resource, ensure you have:

- [ ] An existing App SDK-backed resource with CUE kinds defined
- [ ] A working REST API endpoint (`/apis/<group>/<version>/<resource>`)
- [ ] An app provider registered in the Grafana apps registry
- [ ] REST storage implementation (typically a `legacyStorage` type)

## Step-by-Step Implementation

### Step 1: Add GraphQL Interface to Your App Provider

Modify your app provider to implement the `GraphQLSubgraphProvider` interface:

```go
// pkg/registry/apps/myapp/register.go
package myapp

import (
    graphqlsubgraph "github.com/grafana/grafana-app-sdk/graphql/subgraph"
    "github.com/grafana/grafana-app-sdk/resource"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

type MyAppProvider struct {
    app.Provider
    cfg     *setting.Cfg
    service myapp.Service  // Your existing service
}

// Ensure your provider implements GraphQLSubgraphProvider
var _ graphqlsubgraph.GraphQLSubgraphProvider = (*MyAppProvider)(nil)

// Add this method to your existing provider
func (p *MyAppProvider) GetGraphQLSubgraph() (graphqlsubgraph.GraphQLSubgraph, error) {
    // Get the group version for your app
    gv := schema.GroupVersion{
        Group:   myappv0alpha1.MyResourceKind().Group(),
        Version: myappv0alpha1.MyResourceKind().Version(),
    }

    // Get the managed kinds
    kinds := []resource.Kind{
        myappv0alpha1.MyResourceKind(),
        // Add additional kinds if your app manages multiple resources
    }

    // Create a storage adapter that bridges GraphQL to existing REST storage
    storageGetter := func(gvr schema.GroupVersionResource) graphqlsubgraph.Storage {
        // Only handle your app's resources
        expectedGVR := schema.GroupVersionResource{
            Group:    gv.Group,
            Version:  gv.Version,
            Resource: myappv0alpha1.MyResourceKind().Plural(),
        }

        if gvr != expectedGVR {
            return nil
        }

        // Return a storage adapter that wraps the legacy storage
        legacyStore := p.legacyStorageGetter(gvr)
        if legacyStore == nil {
            return nil
        }

        return &myAppStorageAdapter{
            legacyStorage: legacyStore,
            namespacer:    request.GetNamespaceMapper(p.cfg),
        }
    }

    // Create the subgraph using the helper function
    return graphqlsubgraph.CreateSubgraphFromConfig(graphqlsubgraph.SubgraphProviderConfig{
        GroupVersion:  gv,
        Kinds:         kinds,
        StorageGetter: storageGetter,
    })
}
```

### Step 2: Create a GraphQL Storage Adapter

Create a storage adapter that bridges the GraphQL storage interface to your existing REST storage:

```go
// pkg/registry/apps/myapp/graphql_storage.go
package myapp

import (
    "context"
    "fmt"

    graphqlsubgraph "github.com/grafana/grafana-app-sdk/graphql/subgraph"
    "github.com/grafana/grafana-app-sdk/resource"
    grafanarest "github.com/grafana/grafana/pkg/apiserver/rest"
    "github.com/grafana/grafana/pkg/services/apiserver/endpoints/request"
    "k8s.io/apimachinery/pkg/apis/meta/internalversion"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/labels"
    "k8s.io/apiserver/pkg/registry/rest"
)

// myAppStorageAdapter adapts the existing REST storage to work with GraphQL
type myAppStorageAdapter struct {
    legacyStorage grafanarest.Storage
    namespacer    request.NamespaceMapper
}

// Ensure adapter implements graphqlsubgraph.Storage
var _ graphqlsubgraph.Storage = (*myAppStorageAdapter)(nil)

// Get retrieves a single resource by namespace and name
func (a *myAppStorageAdapter) Get(ctx context.Context, namespace, name string) (resource.Object, error) {
    getter, ok := a.legacyStorage.(rest.Getter)
    if !ok {
        return nil, fmt.Errorf("storage does not support get operations")
    }

    obj, err := getter.Get(ctx, name, &metav1.GetOptions{})
    if err != nil {
        return nil, err
    }

    resourceObj, ok := obj.(resource.Object)
    if !ok {
        return nil, fmt.Errorf("storage returned object that is not a resource.Object: %T", obj)
    }

    return resourceObj, nil
}

// List retrieves multiple resources with optional filtering
func (a *myAppStorageAdapter) List(ctx context.Context, namespace string, options graphqlsubgraph.ListOptions) (resource.ListObject, error) {
    lister, ok := a.legacyStorage.(rest.Lister)
    if !ok {
        return nil, fmt.Errorf("storage does not support list operations")
    }

    // Convert GraphQL list options to Kubernetes list options
    listOptions := &internalversion.ListOptions{}
    if options.LabelSelector != "" {
        selector, err := labels.Parse(options.LabelSelector)
        if err != nil {
            return nil, fmt.Errorf("invalid label selector: %v", err)
        }
        listOptions.LabelSelector = selector
    }
    if options.Limit > 0 {
        listOptions.Limit = options.Limit
    }
    if options.Continue != "" {
        listOptions.Continue = options.Continue
    }

    obj, err := lister.List(ctx, listOptions)
    if err != nil {
        return nil, err
    }

    listObj, ok := obj.(resource.ListObject)
    if !ok {
        return nil, fmt.Errorf("storage returned object that is not a resource.ListObject: %T", obj)
    }

    return listObj, nil
}

// Create creates a new resource
func (a *myAppStorageAdapter) Create(ctx context.Context, namespace string, obj resource.Object) (resource.Object, error) {
    creater, ok := a.legacyStorage.(rest.Creater)
    if !ok {
        return nil, fmt.Errorf("storage does not support create operations")
    }

    if obj.GetNamespace() == "" {
        obj.SetNamespace(namespace)
    }

    created, err := creater.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
    if err != nil {
        return nil, err
    }

    resourceObj, ok := created.(resource.Object)
    if !ok {
        return nil, fmt.Errorf("storage returned object that is not a resource.Object: %T", created)
    }

    return resourceObj, nil
}

// Update updates an existing resource
func (a *myAppStorageAdapter) Update(ctx context.Context, namespace, name string, obj resource.Object) (resource.Object, error) {
    updater, ok := a.legacyStorage.(rest.Updater)
    if !ok {
        return nil, fmt.Errorf("storage does not support update operations")
    }

    obj.SetNamespace(namespace)
    obj.SetName(name)

    updated, _, err := updater.Update(ctx, name, rest.DefaultUpdatedObjectInfo(obj),
        rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
    if err != nil {
        return nil, err
    }

    resourceObj, ok := updated.(resource.Object)
    if !ok {
        return nil, fmt.Errorf("storage returned object that is not a resource.Object: %T", updated)
    }

    return resourceObj, nil
}

// Delete deletes a resource by namespace and name
func (a *myAppStorageAdapter) Delete(ctx context.Context, namespace, name string) error {
    deleter, ok := a.legacyStorage.(rest.GracefulDeleter)
    if !ok {
        return fmt.Errorf("storage does not support delete operations")
    }

    _, _, err := deleter.Delete(ctx, name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
    return err
}
```

### Step 3: Register Your Provider in the Apps Registry

Add your provider to the apps registry and wire set:

```go
// pkg/registry/apps/apps.go
func ProvideRegistryServiceSink(
    // ... existing parameters ...
    myAppProvider *myapp.MyAppProvider,  // Add your provider
    grafanaCfg *setting.Cfg,
) (*Service, error) {
    // ... existing code ...

    providers := []app.Provider{
        playlistAppProvider,
        myAppProvider,  // Add your provider to the list
    }

    // Add feature flag check if needed
    if features.IsEnabledGlobally(featuremgmt.FlagMyAppBackend) {
        providers = append(providers, myAppProvider)
    }

    // ... rest of function ...
}
```

```go
// pkg/registry/apps/wireset.go
var WireSet = wire.NewSet(
    ProvideRegistryServiceSink,
    playlist.RegisterApp,
    myapp.RegisterApp,  // Add your app registration function
    // ... other registrations ...
)
```

### Step 4: Create Your App Registration Function

Ensure you have a registration function that creates and configures your app provider:

```go
// pkg/registry/apps/myapp/register.go
func RegisterApp(
    service myapp.Service,
    cfg *setting.Cfg,
    features featuremgmt.FeatureToggles,
) *MyAppProvider {
    provider := &MyAppProvider{
        cfg:     cfg,
        service: service,
    }

    appCfg := &runner.AppBuilderConfig{
        OpenAPIDefGetter:    myappv0alpha1.GetOpenAPIDefinitions,
        LegacyStorageGetter: provider.legacyStorageGetter,
        ManagedKinds:        myapp.GetKinds(),
        CustomConfig: any(&myapp.MyAppConfig{
            EnableReconcilers: features.IsEnabledGlobally(featuremgmt.FlagMyAppReconciler),
        }),
        AllowedV0Alpha1Resources: []string{myappv0alpha1.MyResourceKind().Plural()},
    }

    provider.Provider = simple.NewAppProvider(apis.LocalManifest(), appCfg, myapp.New)
    return provider
}
```

## Generated GraphQL Schema

Once implemented, your resource will be automatically available in the GraphQL schema with:

### Query Fields

- `myapp_myresource(namespace: String!, name: String!): MyResource` - Get single resource
- `myapp_myresources(namespace: String!): MyResourceList` - List resources

### Types

- `MyResource` - Individual resource type with `metadata` and `spec` fields
- `MyResourceList` - List wrapper with `items` field
- `ObjectMeta` - Standard Kubernetes metadata
- JSON scalar for `spec` (enhanced type mapping coming in future phases)

## Testing Your Implementation

### 1. Verify Registration

Check that your subgraph is registered:

```bash
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { queryType { fields { name } } } }"}'
```

Look for your prefixed fields like `myapp_myresource` and `myapp_myresources`.

### 2. Test Basic Queries

```graphql
# Get a single resource
{
  myapp_myresource(namespace: "default", name: "test-resource") {
    metadata {
      name
      namespace
      creationTimestamp
    }
    spec
  }
}

# List resources
{
  myapp_myresources(namespace: "default") {
    items {
      metadata {
        name
      }
      spec
    }
  }
}
```

### 3. Test with curl

```bash
# Test single resource query
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ myapp_myresource(namespace: \"default\", name: \"test\") { metadata { name } spec } }"
  }'

# Test list query
curl -X POST http://localhost:3000/api/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ myapp_myresources(namespace: \"default\") { items { metadata { name } } } }"
  }'
```

## Troubleshooting

### Common Issues

1. **"Unknown field" errors**: Check that your provider implements `GraphQLSubgraphProvider` and is properly registered
2. **"No storage available" errors**: Verify your `storageGetter` function and `legacyStorageGetter` implementation
3. **Interface conversion errors**: Ensure your storage adapter implements all required methods
4. **Missing fields in schema**: Check that your resource kinds are properly defined and returned by `GetKinds()`

### Debug Logging

The GraphQL system includes extensive debug logging. Look for log messages prefixed with 🔍 to trace:

- Subgraph creation and registration
- Schema generation process
- Storage adapter operations
- Field resolution

### Verification Steps

1. **Check REST API first**: Ensure `/apis/<group>/<version>/<resource>` works
2. **Verify app provider registration**: Check that your provider appears in the apps registry
3. **Test subgraph creation**: Verify `GetGraphQLSubgraph()` returns without errors
4. **Inspect generated schema**: Use GraphQL introspection to see your fields

## Example: Complete Playlist Implementation

For reference, see the complete playlist implementation:

- `grafana/pkg/registry/apps/playlist/register.go` - App provider and GraphQL subgraph
- `grafana/pkg/registry/apps/playlist/graphql_storage.go` - Storage adapter
- `grafana/pkg/registry/apps/apps.go` - Registry integration
- `grafana/pkg/registry/apps/wireset.go` - Wire dependency injection

## Field Naming Convention

Resources are exposed with prefixed field names to avoid conflicts:

- Format: `<app-name>_<resource>` and `<app-name>_<resources>`
- Examples: `playlist_playlist`, `dashboard_dashboard`, `myapp_myresource`
- The prefix is derived from the first part of your app's group name

## Future Enhancements

- **Enhanced Type Mapping**: Phase 3 will include proper GraphQL type generation from CUE specs
- **Cross-app Relationships**: Automatic relationship resolution between resources
- **Advanced Filtering**: Support for complex filtering and search operations
- **Real-time Subscriptions**: GraphQL subscriptions for real-time updates

---

This pattern provides a consistent, scalable way to extend Grafana's GraphQL federation system while reusing existing storage infrastructure and maintaining backward compatibility with REST APIs.
