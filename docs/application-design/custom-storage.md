# Backing a Kind with your own storage

- [The generated Backend interface](#the-generated-backend-interface)
- [Declaring custom storage on an AppManagedKind](#declaring-custom-storage-on-an-appmanagedkind)
- [Wiring an AppInstaller directly](#wiring-an-appinstaller-directly)
- [Caveats](#caveats)
- [Example: a read-only Backend](#example-a-read-only-backend)
- [Handling ListOptions](#handling-listoptions)
  - [The ListOptions struct](#the-listoptions-struct)
  - [Filtering with LabelFilters](#filtering-with-labelfilters)
  - [Filtering with FieldSelectors](#filtering-with-fieldselectors)
  - [Pagination with Limit and Continue](#pagination-with-limit-and-continue)
  - [ResourceVersion](#resourceversion)
  - [Fields you can safely ignore](#fields-you-can-safely-ignore)
  - [Putting it together](#putting-it-together)
- [Handling WatchOptions](#handling-watchoptions)

If your Kind's data doesn't need to (or shouldn't) live in the platform's generic storage — for example, it lives in an existing SQL database, comes from an external API, or is computed on the fly rather than persisted at all — you can back it with your own Go code while still exposing it through the aggregated API server, so that `kubectl get`, `create`, `update`, `delete` and `watch` all work exactly as they would for a CRD-backed Kind.

## The generated Backend interface

When you run `grafana-app-sdk generate` for a Kind, the SDK generates a `<Kind>Backend` interface alongside the usual object, schema, and client code, e.g. for a `Service` Kind. This is on by default; set `codegen: go: customBackend: false` on the Kind in your CUE definition to skip generating it (see [Toggling TypeScript/Go Codegen](../custom-kinds/writing-kinds.md#toggling-typescriptgo-codegen)):

```go
// generated: service_backend_gen.go
type ServiceBackend interface {
    Get(ctx context.Context, identifier resource.Identifier) (*Service, error)
    List(ctx context.Context, namespace string, opts resource.ListOptions) (*ServiceList, error)
    Create(ctx context.Context, obj *Service) (*Service, error)
    Update(ctx context.Context, obj *Service) (*Service, error)
    Delete(ctx context.Context, identifier resource.Identifier) error
    Watch(ctx context.Context, namespace string, opts resource.WatchOptions) (resource.WatchResponse, error)
}
```

For a detailed walkthrough of implementing `List`/`Watch` against `resource.ListOptions`/`resource.WatchOptions` — label/field filtering, pagination with `Limit`/`Continue`, and resource version handling — see [Handling ListOptions](#handling-listoptions) and [Handling WatchOptions](#handling-watchoptions) below.

## Declaring custom storage on an AppManagedKind

Implement the generated interface against whatever backs your data (a database client, an HTTP client for an external API, etc.), then adapt it into a `rest.Storage` with `apiserver.NewCustomStorage` and declare it on the `AppManagedKind` for that Kind, the same place you'd declare a `Reconciler`, `Validator`, or `Mutator`:

```go
myBackend := &sqlServiceBackend{db: db} // your implementation of ServiceBackend

app, err := simple.NewApp(simple.AppConfig{
    Name: "myapp",
    ManagedKinds: []simple.AppManagedKind{{
        Kind:    ServiceKind(),
        Storage: apiserver.NewCustomStorage[*Service, *ServiceList](ServiceKind(), myBackend),
    }},
})
```

When the `AppInstaller` serves this Kind, it checks `AppManagedKind.Storage` for each of the App's managed kinds and, if set, uses it instead of the platform's generic etcd-backed store — no other wiring is needed. (This requires `InitializeApp` to have been called before `InstallAPIs`, which is already the order the SDK's own server setup uses.)

## Wiring an AppInstaller directly

If you're building the `AppInstaller` yourself outside of `simple.App` — for example, wrapping a hand-written `app.App` — you can instead call `installer.SetCustomStorage` directly with a resolver function:

```go
installer.SetCustomStorage(func(kind, version string) (rest.Storage, bool) {
    if kind == "Service" && version == "v1" {
        return storage, true
    }
    return nil, false
})
```

## Caveats

**It is important to keep in mind** that a custom-storage Kind bypasses the platform's generic store entirely, so it doesn't get features like pagination continue-tokens or optimistic-concurrency `resourceVersion` checks for free — your `Backend` implementation is responsible for whatever semantics it wants to provide. It's also currently not possible to declare subresources (like `status`) on a custom-storage Kind without supplying separate `rest.Storage` for each subresource path, since the platform's subresource storage helpers assume the underlying store is the generic one.

## Example: a read-only Backend

Not every Kind needs to support the full set of verbs. A common case is data that is computed on the fly or mirrored from a system you don't want users to write to through this API — for example, a `Service` Kind whose data is read directly from an external inventory system. The generated `Backend` interface still requires all six methods, but `apiserver.UnsupportedWriteBackend[T, L]` provides `Create`, `Update`, `Delete` and `Watch` implementations that return a "not supported" error, so you only need to implement `Get` and `List`:

```go
import (
    "github.com/grafana/grafana-app-sdk/k8s/apiserver"
    "github.com/grafana/grafana-app-sdk/resource"
)

// inventoryServiceBackend is a read-only ServiceBackend backed by an external inventory API.
// Embedding UnsupportedWriteBackend supplies Create, Update, Delete and Watch, all of which
// return an apierrors.NewMethodNotSupported error.
type inventoryServiceBackend struct {
    apiserver.UnsupportedWriteBackend[*Service, *ServiceList]
    inv *inventoryClient
}

func (b *inventoryServiceBackend) Get(ctx context.Context, identifier resource.Identifier) (*Service, error) {
    record, err := b.inv.Lookup(ctx, identifier.Namespace, identifier.Name)
    if err != nil {
        return nil, err
    }
    return serviceFromRecord(record), nil
}

func (b *inventoryServiceBackend) List(ctx context.Context, namespace string, opts resource.ListOptions) (*ServiceList, error) {
    records, err := b.inv.LookupAll(ctx, namespace)
    if err != nil {
        return nil, err
    }
    list := &ServiceList{}
    for _, record := range records {
        list.Items = append(list.Items, *serviceFromRecord(record))
    }
    return list, nil
}
```

Wiring it up is the same as any other `Backend`, just remembering to set `Kind` on the embedded `UnsupportedWriteBackend` (it's used to build the `GroupResource` for the "not supported" errors):

```go
app, err := simple.NewApp(simple.AppConfig{
    Name: "myapp",
    ManagedKinds: []simple.AppManagedKind{{
        Kind: ServiceKind(),
        Storage: apiserver.NewCustomStorage[*Service, *ServiceList](
            ServiceKind(),
            &inventoryServiceBackend{
                UnsupportedWriteBackend: apiserver.UnsupportedWriteBackend[*Service, *ServiceList]{Kind: ServiceKind()},
                inv:                     inventoryClient,
            },
        ),
    }},
})
```

The errors returned by `UnsupportedWriteBackend` are `StatusError`s with HTTP 405 and `StatusReasonMethodNotAllowed`, which `kubectl` and other API clients render the same way they would for any other resource that doesn't support a verb.

The rest of this page uses `inventoryServiceBackend` (and its `List` method) as the running example for implementing filtering, pagination, and resource version handling.

## Handling ListOptions

This section goes into detail on implementing `Backend.List`, covering every field on `resource.ListOptions`: label/field filtering, pagination, and resource version handling.

### The ListOptions struct

```go
type ListOptions struct {
    // ResourceVersion to list at
    ResourceVersion string
    // LabelFilters are a set of label filter strings to use when listing
    LabelFilters []string
    // FieldSelectors are a set of field selector strings to use when listing
    FieldSelectors []string
    // Limit limits the number of returned results from the List call, when >0.
    // The returned ListMetadata SHOULD include the remaining item count, and the page to use for the next call.
    Limit int
    // Continue is the page to continue from when listing. If non-empty, results will begin at the page token,
    // and return up to the Limit amount.
    Continue string
}
```

`Backend.List(ctx, namespace, opts)` receives one of these, built by the `AppInstaller` from the incoming `kubectl get`/`LIST` request (label selector, field selector, `resourceVersion`, `limit`, and `continue` query parameters). `namespace` is passed separately, and is empty for cluster-scoped Kinds or a namespace-unrestricted request.

Unlike the platform's generic etcd-backed store, none of this filtering happens for you — your `Backend` is fully responsible for interpreting every field it wants to support and applying it. Fields you don't support can simply be ignored (see [Fields you can safely ignore](#fields-you-can-safely-ignore) below), but be aware of what that means for correctness.

### Filtering with LabelFilters

`LabelFilters` is a slice, but in practice the platform sends it as a single element containing the full serialized label selector, e.g. `"env=prod,tier!=frontend"` — the same syntax as `kubectl get -l`. Parse it with `k8s.io/apimachinery/pkg/labels`:

```go
import "k8s.io/apimachinery/pkg/labels"

func matchesLabelFilters(obj resource.Object, filters []string) (bool, error) {
    for _, f := range filters {
        selector, err := labels.Parse(f)
        if err != nil {
            return false, err
        }
        if !selector.Matches(labels.Set(obj.GetLabels())) {
            return false, nil
        }
    }
    return true, nil
}
```

`labels.Set` is a `map[string]string`, which is exactly what `resource.Object.GetLabels()` returns, so no conversion is needed beyond the type alias.

If your data source is a SQL database or similar, you'll usually want to push simple equality selectors down into your query rather than filtering in Go after fetching everything. `Selector.Requirements()` breaks a parsed selector into its individual `key`/`operator`/`values` requirements, which you can translate into `WHERE` clauses for the operators your schema can support, falling back to in-memory filtering (via `Matches`) for anything you can't express in SQL:

```go
import (
    "k8s.io/apimachinery/pkg/labels"
    "k8s.io/apimachinery/pkg/selection"
)

// buildLabelWhereClause pushes the equality/inequality requirements of a label selector down
// into a SQL WHERE clause and args, assuming labels are stored in a "labels" JSON/JSONB column.
// Requirements this can't express (e.g. Exists, In with many values) are left for the caller to
// apply afterwards with selector.Matches, using the returned "unhandled" selector.
func buildLabelWhereClause(filters []string) (clause string, args []any, unhandled labels.Selector, err error) {
    unhandled = labels.Everything()
    for _, f := range filters {
        selector, parseErr := labels.Parse(f)
        if parseErr != nil {
            return "", nil, nil, parseErr
        }
        requirements, _ := selector.Requirements()
        for _, req := range requirements {
            switch req.Operator() {
            case selection.Equals, selection.DoubleEquals:
                clause += " AND labels->>? = ?"
                args = append(args, req.Key(), req.Values().List()[0])
            case selection.NotEquals:
                clause += " AND labels->>? != ?"
                args = append(args, req.Key(), req.Values().List()[0])
            default:
                // Fall back to matching this requirement in Go once rows come back.
                unhandled = unhandled.Add(req)
            }
        }
    }
    return clause, args, unhandled, nil
}
```

`b.inv.LookupAll` (or its SQL equivalent) can then run with `clause`/`args` appended to its query, and the caller applies `unhandled.Matches(labels.Set(obj.GetLabels()))` on the resulting rows to cover anything that wasn't pushed down — giving you the performance of server-side filtering for the common case without losing correctness for selector forms you don't want to hand-translate into SQL.

### Filtering with FieldSelectors

`FieldSelectors` works the same way as `LabelFilters`, but for field selectors like `metadata.name=foo` or `status.phase!=Failed`, parsed with `k8s.io/apimachinery/pkg/fields`:

```go
import "k8s.io/apimachinery/pkg/fields"

func matchesFieldSelectors(fieldSet fields.Set, selectors []string) (bool, error) {
    for _, s := range selectors {
        selector, err := fields.ParseSelector(s)
        if err != nil {
            return false, err
        }
        if !selector.Matches(fieldSet) {
            return false, nil
        }
    }
    return true, nil
}
```

Unlike labels, there's no built-in way to turn an arbitrary `resource.Object` into a `fields.Set` — the platform only guarantees `metadata.name` and `metadata.namespace` are selectable by default. If your Kind declares `selectableFields` in its CUE schema for other paths (e.g. `spec.foo`, `status.phase`), your `Backend` is what makes those actually filterable: build a `fields.Set` from the object's `metadata.name`/`metadata.namespace` plus whichever of your declared `selectableFields` you support, e.g.:

```go
func fieldSetFor(obj *Service) fields.Set {
    return fields.Set{
        "metadata.name":      obj.GetName(),
        "metadata.namespace": obj.GetNamespace(),
        "spec.environment":   obj.Spec.Environment,
    }
}
```

### Pagination with Limit and Continue

When `Limit > 0`, the caller wants at most `Limit` items back, plus a `Continue` token in the response if there are more. `Continue` is an opaque string as far as the API server and clients are concerned — your `Backend` defines its format and is the only thing that ever needs to decode it. A common approach is to encode an offset or a "last seen key":

```go
func (b *inventoryServiceBackend) List(ctx context.Context, namespace string, opts resource.ListOptions) (*ServiceList, error) {
    offset := 0
    if opts.Continue != "" {
        var err error
        offset, err = decodeContinueToken(opts.Continue) // your own encoding, e.g. base64(offset)
        if err != nil {
            return nil, apierrors.NewBadRequest("invalid continue token")
        }
    }

    all, err := b.inv.LookupAll(ctx, namespace) // apply label/field filters first
    if err != nil {
        return nil, err
    }

    list := &ServiceList{}
    end := len(all)
    if opts.Limit > 0 && offset+opts.Limit < end {
        end = offset + opts.Limit
    }
    for _, record := range all[offset:end] {
        list.Items = append(list.Items, *serviceFromRecord(record))
    }

    if end < len(all) {
        list.Continue = encodeContinueToken(end)
        remaining := int64(len(all) - end)
        list.RemainingItemCount = &remaining
    }
    return list, nil
}
```

`Continue` and `RemainingItemCount` are fields on the embedded `metav1.ListMeta` in your generated `<Kind>List` type, so you can set them directly on the returned list, or via the `SetContinue`/`SetRemainingItemCount` methods it gets from implementing `metav1.ListInterface`.

A backend that doesn't support pagination can simply ignore `Limit`/`Continue` and always return the full result set — this is safe (if potentially slow for large result sets) since a caller that doesn't get a `Continue` token back knows the list is complete.

### ResourceVersion

`ListOptions.ResourceVersion` asks the `Backend` to list resources "as of" that resource version. This maps to two different behaviors, depending on how your storage tracks change history:

- **If your source has no concept of resource versions** (e.g. it's a live external API with no versioning), it's reasonable to ignore this field entirely and always return current data — just make sure the `ResourceVersion` you set on returned objects/lists is meaningful for your own `Watch` implementation, since watch bookmarks and reconnects depend on it.
- **If your source can time-travel or has its own change-tracking**, treat `ResourceVersion` as the token to fetch a consistent snapshot at, and set the returned `ListObject`'s `ResourceVersion` to whatever version the snapshot corresponds to, so a subsequent `Watch` from that resource version continues correctly.

There's no `ResourceVersionMatch` field on `ListOptions` (unlike `WatchOptions`, see below) — a caller wanting exact/not-older-than semantics expresses that via the raw request, and the SDK's installer intentionally doesn't attempt to translate that nuance into `ListOptions` today, so most `Backend` implementations can treat `ResourceVersion` as "best effort, as of this point."

### Fields you can safely ignore

If your data set is always small and cheap to fetch in full, it's reasonable for `List` to:
- Ignore `Limit`/`Continue` and return everything in one page.
- Ignore `ResourceVersion` and always return current data.

But `LabelFilters`/`FieldSelectors` should generally be respected if your Kind's schema declares `selectableFields`, since `kubectl get -l`/`--field-selector` and any UI built on top of the list endpoint will otherwise silently return incorrect results instead of failing loudly.

### Putting it together

```go
func (b *inventoryServiceBackend) List(ctx context.Context, namespace string, opts resource.ListOptions) (*ServiceList, error) {
    records, err := b.inv.LookupAll(ctx, namespace)
    if err != nil {
        return nil, err
    }

    matched := make([]*inventoryRecord, 0, len(records))
    for _, record := range records {
        obj := serviceFromRecord(record)
        labelOK, err := matchesLabelFilters(obj, opts.LabelFilters)
        if err != nil {
            return nil, apierrors.NewBadRequest(err.Error())
        }
        fieldOK, err := matchesFieldSelectors(fieldSetFor(obj), opts.FieldSelectors)
        if err != nil {
            return nil, apierrors.NewBadRequest(err.Error())
        }
        if labelOK && fieldOK {
            matched = append(matched, record)
        }
    }

    return paginate(matched, opts.Limit, opts.Continue)
}
```

## Handling WatchOptions

`Backend.Watch(ctx, namespace, opts)` receives a `resource.WatchOptions`:

```go
type WatchOptions struct {
    // ResourceVersion is the resource version to target with the call
    ResourceVersion string
    // ResourceVersionMatch is the way to match against the resource version
    ResourceVersionMatch string
    // EventBufferSize determines the size of the watch event buffer
    EventBufferSize int
    // LabelFilters are a set of label filter strings applied to watched resources
    LabelFilters []string
    // FieldSelectors are a set of field selector strings applied to watched resources
    FieldSelectors []string
    AllowWatchBookmarks bool
    TimeoutSeconds      *int64
    // SendInitialEvents is used by streaming ListWatch
    SendInitialEvents *bool
}
```

`LabelFilters`/`FieldSelectors` work exactly as in `ListOptions` — apply the same `labels.Parse`/`fields.ParseSelector` logic to each event you'd otherwise send, and drop events for objects that don't match.

`ResourceVersion`/`ResourceVersionMatch` control where the watch starts: `ResourceVersionMatch` is empty (start watching strictly after the given `ResourceVersion`, or from "now" if `ResourceVersion` is also empty), `"NotOlderThan"` (a consistent-read starting point at least as new as `ResourceVersion` is acceptable, which is friendlier to implement if your backend doesn't track exact ordering), or `"Exact"` (start at exactly that resource version). Most `Backend` implementations backed by a poll-based or non-versioned source can treat any `ResourceVersion` as "start from now" and ignore `ResourceVersionMatch`.

`TimeoutSeconds`, if set, is how long the caller wants the watch connection to stay open before the server closes it (they'll re-establish it). If your `resource.WatchResponse` doesn't close itself after that duration, the caller can hang around longer than expected, so it's worth honoring with a `context.WithTimeout` or a timer that calls `Stop()`.

`AllowWatchBookmarks` and `SendInitialEvents` are used for the watch-list protocol (see [the SDK's watch-list support](../operators.md)); a `Backend` that doesn't implement bookmarks can ignore `AllowWatchBookmarks` — clients that requested them will simply not receive any, which is valid per the Kubernetes watch protocol.
