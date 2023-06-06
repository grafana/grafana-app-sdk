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
$ curl -o kinds/issue.cue https://raw.githubusercontent.com/grafana/grafana-app-sdk/main/docs/tutorials/issue-tracker/cue/issue-v1.cue
```

```cue
package kinds

issue: {
	name: "Issue"
	crd: {}
	codegen: {
		frontend: true
		backend: true
	}
	currentVersion: [0,0]
	lineage: {
		name: "issue"
		schemas: [{
			version: [0,0]
			schema: {
				spec: {
					title: string
					description: string
					status: string
				}
			}
		}]
	}
}
```
[issue.cue with in-line comments](cue/issue-v1.cue)

Alright, let's break down what we just wrote.

Like with go code, any cue file needs to start with a package declaration. In this case, our package is `kinds`. After the package declaration, we can optionally import other CUE packages (for example, `time` if you wanted to use `time.Time` types) using the same syntax as one might with go. After that, you declare fields. With the grafana-app-sdk, we assume that every top-level field is a kind, and adheres to the [kindsys.Custom](https://github.com/grafana/kindsys/blob/df4488cce33697eccba0536970114fff02b81020/kindcat_custom.cue#L106) kind. That's what our `issue` field is--a Custom kind declaration.

Now, we get to the actual definition of our `issue` model:
```cue
issue: {
	name: "Issue"
	crd: {}
	codegen: {
		frontend: true
		backend: true
	}
	currentVersion: [0,0]
	lineage: {} // trimmed here
}
```
Here we have a collection of metadata relating to our model, and then a `lineage` definition, which we'll get to in a moment. Let's break down the other fields first:

| Snippit                               | Meaning                                                                                                                                                                                                                                                                                                                               |
|---------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| <nobr>`name: "Issue"`</nobr>          | This is just the human-readable name, which will be used for naming some things in-code.                                                                                                                                                                                                                                              |
| <nobr>`crd: {}`</nobr>                | This field is an indicator that the kind is expressible as an API Server resource (typically, a Custom Resource Definition). From a codegen perspective, that means that generated go code will be compatible with `resource`-package interfaces. There are fields that can be set within `crd`, but we're ok with its default values |
 | <nobr>`codegen.frontend: true`</nobr> | This instructs the CLI to generate front-end (TypeScript interfaces) code for our schema. This defaults to `true`, but we're specifying it here for clarity.                                                                                                                                                                          |
| <nobr>`codegen.backend: true`</nobr>  | This instructs the CLI to generate back-end (go) code for our schema. This defaults to `true`, but we're specifying it here for clarity.                                                                                                                                                                                              |
| <nobr>`currentVersion: [0,0]`</nobr>  | This is the current version of the Schema to use for codegen (will default to the latest if not present)                                                                                                                                                                                                                              |

ok, now back to that `lineage`. Within the kind system we are extending, `lineage` will be joined with the definition `thema.#Lineage`. A `#Lineage` is a somewhat complicated definition, but almost everything is handled for us by the kind system and thema except for the need to actually define what our **schemas** look like. Within the lineage, schemas are sequential versions that each have a form which is continuous from the last schema (either "minor version" continuous, where forward translation can be done implicitly, or "major version" continous, where there are breaking changes and forward translation has to be explicitly defined using *lenses*, which we won't get into at this point). To read more about how a Thema Lineage works, consult [the Thema docs](https://github.com/grafana/thema/blob/main/docs/overview.md).

Next in the file, we have this bit:
```cue
lineage: {
    name: "issue"
    schemas: [{
        version: [0,0]
        schema: {
            spec: {
                title: string
                description: string
                status: string
            }
        }
    }]
}
```
The first field, which is the lineage name, _must_ be an all-lowercase version of the kind's name. 
Future versions of kindsys or the codegen may allow you to omit this field.
```cue
schemas: [{
    version: [0,0]
    schema: {
        spec: {
            title: string
            description: string
            status: string
        }
    }
}]
```
Each element in `schemas` is a new schema. Schemas are ordered by version, with each schema requiring a `version` field set as
```[major,minor]```. The first schema must always have a version of `[0,0]`, and subsequent schemas must bump that version 
with either the next major or minor version.

After that, we have the actual `schema` field.
```cue
spec: {
	title: string
	description: string
	status: string
}
```
But wait, why do we have a `spec` key here? This is an important restriction that's imposed by grafana's kind system, not by Thema itself. 
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
