# Resource Objects

The core kernel of working with the SDK's storage API is the `resource.Object` interface. 
An instance of `Object` represents a distinct resource in the storage system, and it what is used in all core which interacts with the storage, 
including stores, operators, and clients. An Object is considered to have three main components:
1. Spec - this is the main body of the objects, and where all user-editable data is stored
2. Subresources - currently, the only supported subresource when using the kubernetes client is `Status`. Subresources are non-spec, non-metadata top-level objects that are returned on reads, but must be written separately from a write to Spec.
3. Metadata - this is split into three distinct kinds of metadata:
    * Static Metadata - metadata which is immutable and can be used to uniquely identify the resource.
    * Common Metadata - metadata which is common across all resources of all kinds
    * Custom Metadata - metadata which is unique to this kind or even this specific kind version

The `resource.Object` interface defines methods used for interacting with these components. 
Additionally, it defines a function used to unmarshal byte payloads for each of these components into the Object itself.

## Implementing resource.Object

### Using the CLI

By far the easiest way to implment `resource.Object` for your project(s) is to use the CLI's `generate` command, 
which takes in a kind written in CUE and outputs all the go code you'll need in your project, as well as CRD file(s) you can use to define the resource(s) in kubernetes. 

See [Code Generation](code-generation.md) for more details on using the CLI for codegen.

### By Hand

You can implement the interface by hand if you like, but keep in mind a few things:
* `Unmarshal` must be able to handle any valid `WireFormat` and any valid version of the resource that can be stored.
* `SpecObject()` and all resources in the map returned by `Subresources()` are marshaled as-is to the storage layer, 
    so they should be shaped in a way that they will be accepted (this will hopefully change soon).

As this SDK is still **experimental**, the `resource.Object` interface may go through further evolutions, 
so it's generally advisable to use the codegen, which will always generate compliant code. 
