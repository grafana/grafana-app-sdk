# Defining Our Kinds & Schemas

The first thing we want to think about for our project is what our data looks like. In our case, we want to track issues, so we want some kind of `Issue` object which would have the following properties:
* Name
* Description
* Status (open, in-progress, closed, etc.)

Every app has to expose one or more version for its types, and other related APIs. Kinds and other custom endpoints are always grouped within a version, so we also need to have a version for our `Issue` type. 
This allows us to later make changes to our `Issue` schema without breaking existing clients, by publishing a new version.

## Versions, Kinds, and Schemas in CUE

A Kind is a collection of data which wraps a Schema for a resource type. A Version is an API version which contains one or more kinds. 
Apps in app platform will always expose an API for each defined version, and each Kind in the version will get routes for creating, reading, updating, deleting, patching, listing, and watching instances of the Kind (often called "resources" or "objects").
While the SDK technically will work with any objects you define so long as they implement the proper interfaces, by far the easiest way to work with it (and the way that takes advantage of codegen and schema adaptability mentioned above) is by defining your kinds in CUE, and generating code for working with them. As this is the most-intended and easiest way to use the SDK, that's what we're going to do here. Let's start with what our CUE kind will look like for our `Issue`, and then break down what exactly all the fields mean.

Either copy-and-paste the following CUE into a file called `kinds/issue.cue`, or pull down [`cue/issue-v1.cue`](cue/issue-v1.cue) by running:
```shell
curl -o kinds/issue.cue https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/cue/issue-v1.cue
```

```cue
package kinds

issueKind: {
	kind: "Issue"
	pluralName: "Issues"
	scope: "Namespaced"
	codegen: {
		ts: {
			enabled: true
		}
		go: {
			enabled: true
		}
	}
}

issuev1alpha1: issueKind & {
	schema: {
		spec: {
			title: string
			description: string
			status: string
		}
	}
}
```
[issue.cue with in-line comments](cue/issue-v1.cue)

Alright, let's break down what we just wrote.

Like with Go code, any cue file needs to start with a package declaration. In this case, our package is `kinds`. After the package declaration, we can optionally import other CUE packages 
(for example, `time` if you wanted to use `time.Time` types) using the same syntax as one might with Go. After that, you declare fields. 
In this case, we have two types we're declaring: `issueKind` and `issuev1alpha1`. When we only have one version, these can really be the same type, 
but it becomes easier to publish additional versions if you have them split out. 
`issueKind` is the information about our `Issue` kind which does not change between versions, and `issuev1alpha1` is `issueKind` + version-specific information. 
In this case, the only version-specific information we have is our schema.

So, what's going on with `issueKind`? We've got a few fields here that describe our kind information, which we can break down:

| Snippit                                 | Meaning                                                                                                                                                                                                           |
|-----------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| <nobr>`kind: "Issue"`</nobr>            | This is just the human-readable name, which will be used for naming some things in-code.                                                                                                                          |
| <nobr>`pluralName: "Issues"`</nobr>     | This is the plural name of our kind, which is optional and will default to the value of `kind` + `s`.                                                                                                             |
| <nobr>`scope: "Namespaced"`</nobr>      | This is an optional field, which designates whether instances of the kind (resources) are created on a per-tenant basis (Namespaced) or globally (Cluster). It defaults to Namespaced if you leave the field out. |
| <nobr>`codegen.ts.enabled: true`</nobr> | This instructs the CLI to generate front-end (TypeScript interfaces) code for our schema. This defaults to `true`, but we're specifying it here for clarity.                                                      |
| <nobr>`codegen.go.enabled: true`</nobr> | This instructs the CLI to generate back-end (go) code for our schema. This defaults to `true`, but we're specifying it here for clarity.                                                                          |
| <nobr>`current: "v1"`</nobr>            | This is the current version of the Schema to use for codegen.                                                                                                                                                     |

Now, we still have `issuev1alpha1`, so what's its deal? In an app, we have one or more API versions which are exposed, 
and each version has one or more kinds as part of it. Kinds with the same name should be the same logical kind 
(in that they are treated as equivalent objects logically, and conversion is possible between each version of the kind), 
but things like their schema and a few other capabilities we'll delve into later can differ. 
This is how we can evolve our kinds as we need to add more functionality or data (or change data format) 
without breaking compatibility for existing users--we can create a new version of the kind with an altered schema.

> [!NOTE]
> The default first version when building an app is `v1alpha1`. This follows the [app manifest versions guidelines](../../app-manifest.md#versions), 
> where unstable versions are `v<number>alpha<number>` or `v<number>beta<number>`, and stable versions are simply `v<number>`. 
> If you like, you can start on a stable version, but we recommend your first pass use an unstable one so you can more freely change it.

Since all we need in the `v1alpha1`-specific version of `Issue` (aside from the common information in `issueKind`) is its `schema`, 
We declare `issuev1alpha1` to be a combination of `issueKind` plus our `schema` field. Our `schema` itself looks like:
```cue
spec: {
	title: string
	description: string
	status: string
}
```
But wait, why do we have a `spec` key here? That wasn't a requirement in `def.cue`! 
This is an important restriction imposed by the SDK codegen when defining the spec of an API server resource, to promote consistency across all App kinds. 
The definition for this restriction actually also lives in `def.cue`, [it's called Schema there](https://github.com/grafana/grafana-app-sdk/blob/main/codegen/cuekind/def.cue#L24).
Each top-level field in the resource is considered to be a unique component of the resource when expressed in an API Server. 
The `spec` component is the actual definition of our object's body, but there are two other components that are included implicitly even if not declared: 
`metadata` and `status`. `metadata` includes all the common metadata associated by the kind system and the app-sdk to _all_ kinds. 
The SDK takes responsibility for mapping this metadata to and from how it may be expressed in the underlying API server. 
You can add additional, kind-specific metadata by using the `metadata` key, but be aware that it cannot conflict with existing common metadata fields, 
and can only be of a `string` value. `status` contains state information, and, by default, will have optional fields for 
operators to add the last computed lifecycle state for the resource, and a section reserved for future use. 
This section is typically not editable by users, and only updated by operator/controller processes.

For our purpose, we don't need additional metadata, and don't need to track anything special in the `status`, so we only need a `spec`.

Now that we've defined our kind, it's time to [Generate Kind Code](03-generate-kind-code.md)

### Prev: [Initializing Our Project](01-project-init.md)
### Next: [Generate Kind Code](03-generate-kind-code.md)
