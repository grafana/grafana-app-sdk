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
* **List** - List all object in a namespace with provided filters

It is important to keep in mind that, like a typical key-value store, `Update` overwrites the entire object, so the standard pattern for usage is get-and-update to ensure that you don't erase fields. To update only specific parts of an object, use `Client.Patch`.

