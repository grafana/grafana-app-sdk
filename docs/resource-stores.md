# Resource Stores

While you can use a `resource.Client` directly to interact with the API server, the `resource` package also offers several types which allow you to treat the API server as a storage system and interact with it via key-value store paradigms. These use `resource.Client` under the hood, but handle some of the more annoying or repetitive tasks that you often need to do, and introduce some streamling and extra opinionated functionality.

There are three types of store provided by the `resource` package, but be aware that `SimpleStore` is currently deprecated and will not have new funtionality added to it.

## TypedStore

`resource.TypedStore` allows you to work with a provided kind, using the type directly instead of interfacing with `resource.Object`. It provides the following methods:
* **Get** - Gets an existing object by `resource.Identifier`
* **Add** - Creates a new object (errors if the object exists)
* **Update** - Updates an existing object (errors if the object doesn't exist)
* **Upsert** - Updates an existing object, or creates the object if it doesn't exist
* **UpdateSubresource** - Updates a subresource of the object--this must be done separately from **Update**, which does not update subresources
* **Delete** - Deletes an existing object (errors if the object doesn't exist)
* **ForceDelete** - Deletes an existing object, does not error if the object doesn't exist
* **List** - List all object in a namespace with provided filters. Valid filters are [kubernetes label selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors)

It is important to keep in mind that, like a typical key-value store, `Update` overwrites the entire object, so the standard pattern for usage is get-and-update to ensure that you don't erase fields. To update only specific parts of an object, use `Client.Patch`.

An example of using a TypedStore to list all objects that match a selector and then update them:
```go
store, err := resource.NewTypedStore[*v1.MyObject](v1.Kind(), clientGenerator)
if err != nil {
    panic(err)
}
ctx := context.TODO()
// Get resources of v1.MyObject type in all namespaces that have the labels `environment=dev` and `live=true`
list, err := store.List(ctx, resource.NamespaceAll, "environment=dev", "live=true")
if err != nil {
    panic(err)
}
// store.List returns a resource.TypedList[*v1.MyObject], so we can iterate through list.Items 
// to get the *v1.MyObject type instead of using list.GetItems() to get a list of resource.Object types
for _, item := range list.Items {
    // Since we have the actual type, we can directly access fields on it without needing to cast
    item.ObjectMeta.Labels["live"] = false
    // The first return of the Update method is the updated object, but we can discard that because we don't care about it
    if _, err := store.Update(ctx, item.GetStaticMetadata().Identifier(), item); err != nil {
        panic(err)
    }
}
```
The most useful aspect of this example is that we can directly work with the returned `*v1.MyObject` instances, instead of a `resource.Object` that is typically returned by a Client.

## Store

`resource.Store` is a generic store that allows for working with any kind, not just a single one like `TypedStore`. This comes with the downside that arguments are returned types now use `resource.Object` instead of concrete types. If you're only working with a single kind, prefer using `resource.TypedStore` (you may want to consider multiple `resource.TypedStore` instances for multiple kinds as well if you find the workflow simpler, as the reasource overhead isn't that signficant, considering that `resource.Store` still utilizes a `resource.Client` instance per kind under the hood). 

In order to work with multiple kinds properly, each kind must be registered with the store. This can be done at any time prior to using the kind in an argument to a store method. If you attempt to work with a kind which is not registered, the store will return an error. You can register one or more kinds with `Register` and `RegisterGroup`. You can also optionally supply any number of kind groups to be registered when creating the store with `NewStore`.

`resource.Store` provides the same methods as `resource.TypedStore`, with slightly more complex signatures, as it also requires a `kind` string to identify the kind of the object for `Get`, `List`, `UpdateSubresource`, `Delete` and `ForceDelete`. It also provides a few additional methods:
* **SimpleAdd** - Creates a new object, but accepts a `kind` and `resource.Identifier`, that is uses to overwrite whatever is in the provided object's `StaticMetadata`. This is useful for copying an object, or when you only want to work with an object's `spec` without worrying about metadata.
* **Client** - Returns a `resource.Client` instance used by the store for the provided kind. This will only work for kinds which have been registered with the store.

Here we have the same example as with `resource.TypedStore`, only we want to work with a set of kinds:
```go
store, err := resource.NewStore(clientGenerator, kindSet)
if err != nil {
    panic(err)
}
ctx := context.TODO()
// We're going to combine the list of all items from each kind in the kindSet, 
// as store.List() returns a `resource.ListObject` interface anyway, we don't lose type information, 
// because GetItems() returns a generic `[]resource.Object`.
// Since we don't need the list metadata, we can drop it and only work with the []resource.Object GetItems() response.
combined := make([]resource.Object, 0)
for _, kind := range kindSet.Kinds() {
    // Get resources of the kind in all namespaces that have the labels `environment=dev` and `live=true`
    list, err := store.List(ctx, kind.Kind(), resource.NamespaceAll, "environment=dev", "live=true")
    if err != nil {
        panic(err)
    }
    combined = append(combined, list.GetItems()...)
}
for _, item := range combined {
    // Since we don't have the actual type, we either have to cast or call resource.Object methods
    labels := item.GetLabels()
    labels["live"] = false
    item.SetLabels(labels)
    // The first return of the Update method is the updated object, but we can discard that because we don't care about it
    if _, err := store.Update(ctx, item.GetStaticMetadata().Identifier(), item); err != nil {
        panic(err)
    }
}
```
We can see how this can make working with many kinds at once a simpler process, particularly if you only care about metadata.

## SimpleStore

> [!WARNING]
> `resource.SimpleStore` is deprecated, and will not receive futher updates except to support existing functionality.
> It may be removed in a future version of the `grafana-app-sdk`.

`resource.SimpleStore` is used for interacting with objects when you only care about working with the `Spec` of the resource. To this end, the ability to manipulate metadata is more limited, but methods accept just the `Spec` type, rather than the whole object. `SimpleStore` provides fewer methods than `TypedStore`, missing the `Upsert` and `ForceDelete` methods. The `TypedStore` example is possible to do in `SimpleStore`, but becomes more convoluted because it is performing metadata manipulation. Instead, prefer using `TypedStore` anywhere you may want to use `SimpleStore`. If there is a use-case where `SimpleStore` fits your needs more than `TypedStore`, please [open an issue](https://github.com/grafana/grafana-app-sdk/issues) or [start a discussion](https://github.com/grafana/grafana-app-sdk/discussions) on the topic to let us know and asses the future path for `SimpleStore`.