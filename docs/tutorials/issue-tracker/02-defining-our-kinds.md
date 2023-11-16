# Defining Our Kinds & Schemas

The first thing we want to think about for our project is what our data looks like. In our case, we want to track issues, so we want some kind of `Issue` object which would have the following properties:
* Name
* Description
* Status (open, in-progress, closed, etc.)

We may want to give our `Issue` object more properties in the future, but that's actually not a showstopper with the way the SDK works, as we'll see a bit later. For now, just know that the goal of the SDK is to get you up and running, without you needing to figure out what the final state of your object's schema will look like before you can proceed.

## Kinds and Schemas in CUE

A Kind is a collection of data which wraps a set of Schemas for a resource type.
While the SDK technically will work with any objects you define so long as they implement the proper interfaces, by far the easiest way to work with it (and the way that takes advantage of codegen and schema adaptability mentioned above) is by defining your kinds in CUE, and generating code for working with them. As this is the most-intended and easiest way to use the SDK, that's what we're going to do here. Let's start with what our CUE kind will look like for our `Issue`, and then break down what exactly all the fields mean.

Either copy-and-paste the following CUE into a file called `kinds/issue.cue`, or pull down [`cue/issue-v1.cue`](cue/issue-v1.cue) by running:
```shell
curl -o kinds/issue.cue https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/cue/issue-v1.cue
```

```cue
package kinds

issue: {
	kind: "Issue"
        group: "issue-tracker-project"
	apiResource: {}
	codegen: {
		frontend: true
		backend: true
	}
	current: "v1"
	versions: {
		"v1": {
			schema: {
				spec: {
					title: string
					description: string
					status: string
				}
			}
		}
	}
}
```
[issue.cue with in-line comments](cue/issue-v1.cue)

Alright, let's break down what we just wrote.

Like with Go code, any cue file needs to start with a package declaration. In this case, our package is `kinds`. After the package declaration, we can optionally import other CUE packages (for example, `time` if you wanted to use `time.Time` types) using the same syntax as one might with Go. After that, you declare fields. With the grafana-app-sdk, we assume that every top-level field is a kind, and adheres to the [CUE Kind definition](https://github.com/grafana/grafana-app-sdk/blob/main/codegen/cuekind/def.cue#L82). That's what our `issue` field is--a Kind declaration.

Now, we get to the actual definition of our `issue` model:
```cue
issue: {
	kind: "Issue"
	apiResource: {}
	codegen: {
		frontend: true
		backend: true
	}
	current: "v1"
	versions: {} // trimmed here
}
```
Here we have a collection of metadata relating to our model, and then a `versions` definition, which we'll get to in a moment. Let's break down the other fields first:

| Snippit                               | Meaning                                                                                                                                                                                                                                                                                                                               |
|---------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| <nobr>`kind: "Issue"`</nobr>           | This is just the human-readable name, which will be used for naming some things in-code.                                                                                                                                                                                                                                              |
| <nobr>`apiResource: {}`</nobr>         | This field is an indicator that the kind is expressible as an API Server resource (typically, a Custom Resource Definition). From a codegen perspective, that means that generated go code will be compatible with `resource`-package interfaces. There are fields that can be set within `apiResource`, but we're ok with its default values |
 | <nobr>`codegen.frontend: true`</nobr> | This instructs the CLI to generate front-end (TypeScript interfaces) code for our schema. This defaults to `true`, but we're specifying it here for clarity.                                                                                                                                                                          |
| <nobr>`codegen.backend: true`</nobr>  | This instructs the CLI to generate back-end (go) code for our schema. This defaults to `true`, but we're specifying it here for clarity.                                                                                                                                                                                              |
| <nobr>`currentVersion: [0,0]`</nobr>  | This is the current version of the Schema to use for codegen (will default to the latest if not present)                                                                                                                                                                                                                              |

ok, now back to `versions`. In the CUE kind, `versions` is a map of a version name to an object containing meta-information about the version, and its `schema`. 
The map is unordered, so there is no implicit or explicit sequential relationship between versions in your Kind definition, but consider naming conventions that portray the evolution of the kind (for example, `v1`, `v1beta`, `v2`, etc. like kubernetes' kind versions). The `current` top-level Kind attribute declares which version in this map is the current, 
or preferred version. Note that this does not need to be the latest version. This is also the version that will be indicated to be "stored" when generating a CRD from the Kind.

Inside our `"v1"` version, we have the actual `schema` field:
```cue
spec: {
	title: string
	description: string
	status: string
}
```
But wait, why do we have a `spec` key here? That wasn't a requirement in `def.cue`! This is an important restriction imposed by the SDK codegen when defining an API-server compatible resource (`apiResource` in the kind). The definition for this restriction actually also lives in `def.cue`, [it's called Schema there](https://github.com/grafana/grafana-app-sdk/blob/main/codegen/cuekind/def.cue#L24).
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
