# Using Kinds

Once you have written one or more kinds, you need to be able to use them in your app. 

## Go Code

Most of the grafana-app-sdk code deals with kinds in terms of `resource.Kind` and `resource.Object`. 
`resource.Kind` is the collection of kind metadata, a generator for new instances of the kind (a `resource.Object` implementation), and contains a set of `resource.Codec` implementations for encoding/decoding the kind to/from a specific wire format. `resource.Object` is a superset of several kubernetes interfaces used for working with objects, and some additional methods that are useful to users and the app-sdk.

Generally, `resource.Kind` (or sometimes a subset, `resource.Schema`, which is missing Codec information) is ingested by operators and clients, and `resource.Object` is returned by clients, and consumed by watchers/informers. That is, you'll mostly be supplying a `resource.Kind` to a type or function, and be handling `resource.Object`s.

If you codegen your kinds using `grafana-app-sdk generate`, you'll have access to a `resource.Kind` instance with the generated `Kind()` method in the generated kind/version package, and an exported type named after the kind will implement `resource.Object` for you.

### Non-Generated Kinds

If you don't use the codegen, either because you cannot or do not wish to, you can always implement the `resource.Object` interface yourself and create your own `resource.Kind` instance. To make this simpler, the grafana-app-sdk `resource` package contains a few `resource.Object` implementations with type parameters that allow you to create generic kinds with specific struct spec (and subresources).

`resource.TypedSpecObject[T]` is an implementation of `resource.Object` which has a spec of type `T`. The only restriction with this object is that it doesn't contain any subresources, so it's only useful when working with the spec and metadata of an object. It can be marshaled to or unmarshaled from kubernetes JSON without the need for any special logic.

`resource.TypedSpecStatusObject[T,S]` is an implementation of `resource.Object` which has a spec of type `T`, and a status subresource of type `S`. This is an extension on `resource.TypedSpecObject[T]` to include a status subresource. It can be marshaled to or unmarshaled from kubernetes JSON without the need for any special logic.

`resource.TypedObject[T,S]` is an implementation of `resource.Object` which has a spec of type `T`, and a catalog of subresources of type `S`. The top-level fields in `S` should all correlate to subresources for the object. A simple `map[string]any` implementation of this is `resource.MapSubresourceCatalog`. It can be marshaled to or unmarshaled from kubernetes JSON without the need for any special logic, provided the subresource catalog type fits the criteria.

For untyped data, there is `resource.UntypedObject`, which can be used for handling objects without a corresponding go type. The `spec` is a `map[string]any`, and all subresources are in a `map[string]json.RawMessage`.

You may also choose to implement `resource.Object` yourself.

The easiest way to declare a Kind is the way the codegen also does it: 
```go
myKind := resource.Kind{
    Schema: resource.NewSimpleSchema(group, version, emptyObject, resource.WithKind(kindName)),
    Codecs: map[resource.KindEncoding]resource.Codec{
        resource.KindEncodingJSON: resource.NewJSONCodec(),
        // Additional encodings and codecs as required
    },
}
```

**See also:** [Resource Objects](../resource-objects.md)

## TypeScript

There are no intrinsic interfaces to satisfy with TypeScript, but the grafana-app-sdk will generate some TypeScript interfaces for using with a kubernetes API server based on your kind.