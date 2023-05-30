# Resource Stores

The `resource` package offers several different `Store` objects for interacting with objects in your storage. 
These stores are built on top of whatever `resource.Client` you use to talk to your storage API server, 
and present a way of interacting with objects that follows a key/value store interface. 
There are currently three types of Store, and they all have the same set of methods, but work with different inputs and/or return values:

* `SimpleStore` - SimpleStore uses it's own implementation of `resource.Object`, called `SimpleStoreResource`, and assumes that you never want to manipulate object metadata, 
    instead accepting only creates/updates to the spec object and subresources. Interaction with metadata is limitd to `CommonMetadata`, and only via `ObjectMetadataOption` 
    functions which can be passed to add and update calls.
* `TypedStore` - TypedStore works with a concrete implementation of `resource.Object` which you provide when constructing it. 
    It always accepts and returns objects of the provided type, making it easy to work with without needing to do type casts. 
    It allows you to manipulate the entire objects, unlike `SimpleStore`, at the cost of some added complexity.
* `Store` - Store is the fully generic Store, working with any resources which implement `resource.Object`, not just the one you bind to it at construction. 
    To that end, it is the most cumbersome to use, as if you need to access the underlying types you have to do type casting, 
    but it allows you to work with resources of different kinds using the same store.