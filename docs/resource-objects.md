# Resource Objects

The core kernel of working with the SDK's libraries is the `resource.Object` interface. An instance `resource.Object` 
(henceforth referred to as `Object` for simplicity) represents an instance of a [Kind](./custom-kinds/README.md), 
and is typically populated with data for that instance. An `Object` implementation is composed of three main components:
1. Spec - this is the main body of the object, and where all user-editable data is stored. Read more about `spec` in [Kinds](./custom-kinds/README.md).
2. Subresources - these are additional sub-objects which are typically only mutated by operators. The clearest example of this is the `status` subresource, typically used to track operator state when handling the object. Currently, when using CRDs with a [frontend-only](./application-design/README.md#frontend-only-applications) or [operator-based](./application-design/README.md#operator-based-applications), only `status` and `scale` subresources are supported.
3. Metadata - these are fields that always exist for all objects, such as `name` or `labels`. Metadata can be accessed on a per-field basis, or in SDK-specific groupings:
    * Static Metadata - metadata which is immutable upon creation of the object and can be used to uniquely identify the resource
    * Common Metadata - a subset of general metadata that _also_ includes non-kubernetes, app-platform specific metadata, such as `updateTimestamp` and `createdBy`. These additional metadata fields are normally encoded in `annotations` in the kubernetes metadata.

The `Object` interface defines methods for accessing and all standard kubernetes metadata, accessing and setting the StaticMetadata and CommonMetadata objects, 
and accessing and setting `spec` and subresources.

## Implementing resource.Object

### Using the CLI

By far the easiest way to implement `resource.Object` for your project(s) is to use the CLI's `generate` command, 
which takes in a kind written in CUE and outputs all the go code you'll need in your project, as well as CRD file(s) you can use to define the resource(s) in kubernetes. 
For more information on writing kinds in CUE for codegen, see [Writing Kinds](./custom-kinds/writing-kinds.md).

A generated `Object` implementation will also contain extra getters and setters for all "custom" metadata defined in your CUE kind, 
and getters and setters for app platform non-kubernetes metadata (such as `updateTimestamp`), which will properly encode the custom metadata 
into the kubernetes annotations metadata.

### By Hand

You can implement the interface by hand if you like, but keep in mind a few things:
* There are a _lot_ of getters/setters for metadata--the easiest way to implement these is to embed `k8s.io/apimachinery/pkg/apis/meta/v1.TypeMeta` and `k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta` at the root of your struct.
* If your `Object` implementation doesn't easily convert to kubernetes JSON with `json.Marshal`/`json.Unmarshal`, you'll need to define your own `resource.Codec` to use in a `resource.Kind` for marshal/unmarshal process.
* `resource.TypedObject` and `resource.UntypedObject` may serve your needs if you're just trying to handle runtime-provided spec or subresource information

As this SDK is still **experimental**, the `resource.Object` interface may go through further evolutions, 
so it's generally advisable to use the codegen (or `resource.TypedObject`/`resource.UntypedObject`), which will always generate compliant code. 
