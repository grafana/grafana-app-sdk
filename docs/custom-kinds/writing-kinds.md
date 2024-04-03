# Writing Kinds

The preferred way of writing kinds for use with codegen provided by the `grafana-app-sdk` CLI is using CUE (support for other input types in the future). If you are familiar with CUE, the base definition of a kind [exists in codegen/cuekind/def.cue](https://github.com/grafana/grafana-app-sdk/blob/main/codegen/cuekind/def.cue). However, given that a CUE definition may not be the easiest to understand, especially if you lack familiarity with CUE, we will go in-depth into writing kinds in CUE here. _No prior CUE knowledge or experience is required_. 

Defining a kind can be thought of as being split into two parts: the kind metadata, and the schemas for each version. A simple kind, without any versions (which makes it invalid, but it's a place to start) would look like this:
```cue
foo: {
    // kind is the kind name. It must be capitalized by convention
    kind: "Foo"
    // group is the group the kind belongs to, typically the name of the app. 
    // When generating the CRD and resource.Kind, the group will be appended with '.ext.grafana.com'
    group: "myapp"
    // apiResource tells the codegen that this is an API resource. 
    // Optionally, one can specify additional options in this object.
    // For non-API resource kinds, see later section.
    apiResource: {}
}
```

> [!TIP]
> You can generate a kind with descriptive comments of all fields with `grafana-app-sdk project kind add <KindName>`

## Schemas

To complete the kind, it needs a version and a schema for that version. These slot into the kind like so:
```cue
foo: {
    // ... existing fields ...

    // We have to specify the currentVersion, even if there's only one version
    currentVersion: "v1"

    // Collection of all versions for the kind, as a map of version string => version details
    versions: {
        // This is our "v1" version
        "v1": {
            // schema is the only required field for a version, and contains the schema for this version of the kind
            schema: {
                // ... schema goes here ...
            }
        }
    }
}
```

What is the `schema`? It's the template for the data. If you're familiar with OpenAPI, the schema is rendered into a subset of OpenAPI when converted into a Custom Resource Definition for the kubernetes API server. In CUE, a schema follows the pattern of [a definition](https://cuelang.org/docs/tour/types/defs/), which declares field names and types. Something like:
```cue
{
    field1: string
    field2: int64
    field3: bool
    field4: float64
}
```
Is the format of a definition. The declarative style is similar to TypeScript, and it uses go types. You can add additional restrictions as well:
```cue
{
    positiveNumber: int64 & >0
}
```

The `schema` of an API resource also has a few restrictions on it: there _MUST_ be a `spec` field (and this field _SHOULD_ be a struct type), and any other top-level field in the `schema` will be considered to be a subresource within the kubernetes API. At present, only `status` and `scale` are supported for Custom Resource Definitions (CRDs), so other subresource fields will not be supported in your CRD.

With all that, let's complete our simple kind:
```cue
foo: {
    kind: "Foo"
    group: "myapp"
    apiResource: {}
    currentVersion: "v1"
    versions: {
        "v1": {
            schema: {
                spec: {
                    stringField: string
                    intField: int64
                }
            }
        }
    }
}
```

For this to be a valid CUE file, it needs a `package` which should be the directory in which it lives. You'll also need to have initialized a CUE module. `grafana-app-sdk project init` does this for your automatically (creating the `kinds` directory), but if you want to do it yourself, you'll need to install CUE and run `cue mod init`.

Our final CUE file looks like:
```cue
package kinds

foo: {
    kind: "Foo"
    group: "myapp"
    apiResource: {}
    currentVersion: "v1"
    versions: {
        "v1": {
            schema: {
                spec: {
                    stringField: string
                    intField: int64
                }
            }
        }
    }
}
```

## Generating Code

We now have a valid kind! If you save this as a CUE file (`.cue`) in your project (the default directory for parsing kinds is `./kinds`), you can now generate code and a CRD file for your kind. To do so, make sure you have the `grafana-app-sdk` CLI installed (you can download a binary for your distribution on the [releases](https://github.com/grafana/grafana-app-sdk/releases) page, build the binary from the repo with `make build`, or use `go install` with the cloned repo (there is a known issue with `replace` in the `go.mod` that prevents `go install` working from a remote source)). Now you can run
```shell
grafana-app-sdk generate
```
(if you saved your CUE file to a directory different than `./kinds`, add `-c <CUE directory>`)

Generated code by default ends up in three different places (these directories can be customized with CLI flags, use `grafana-app-sdk generate --help` to display them):
* `pkg/generated/resource/foo/v1`
* `plugin/src/generated/foo/v1`
* `definitions/`

### `pkg/generated/resource`

All generated go code ends up in `pkg/generated/resource/<kind name>/<kind version>`. For each kind, there are at least six files that are generated (at least six, because each subresource generates its own go file):
* `foo_codec_gen.go` contains information for the kind to use to encode/decode the go type
* `foo_metadata_gen.go` is a file that exists for legacy support, and will be eventually removed from codegen
* `foo_object_gen.go` is a file that contains the `Foo` type, which implements `resource.Object`. For more information on `resource.Object`, see [Using Kinds](./using-kinds.md) or [Resource Objects](../resource-objects.md)
* `foo_schema_gen.go` is a file that contains functions for returning a `resource.Kind` and `resource.Schema` (`Kind()` and `Schema()` respectively). For more details on `resource.Kind`, see [Using Kinds](./using-kinds.md)
* `foo_spec_gen.go` is a file that contains a type declaration for the `Spec` type, as defined in our CUE. It is used by `Foo` in `foo_object_gen.go`
* `foo_status_gen.go` is a file that contains a type declaration for the `Status` type, as defined in our CUE. We didn't define a `status` subresource, but there is always a "basic" status subresource for each app platform object that contains some generic data. You can see its definition either in the go code, or [as part of the CUE definition of a schema](https://github.com/grafana/grafana-app-sdk/blob/main/codegen/cuekind/def.cue#L42-L67).

Additional `foo_x_gen.go` files will be generated for each subresource in your schema (and will be added as a field in `Foo`).

To use this generated code in your project, see [Using Kinds](./using-kinds.md), [Operators & Event-Based Design](../operators.md), [Resource Objects](../resource-objects.md), or [Resource Stores](../resource-stores.md).

### `plugin/src/generated`

All generated TypeScript code ends up in `plugin/src/generated/<kind name>/<kind version>`. For each kind, there are at least three files that are generated (at least three, because each subresource generates its own TypeScript file):
* `foo_object_gen.ts` contains the `Foo` interface, which is compatible with the kubernetes API server definition of the `Foo` kind for that version. 
* `types.spec.gen.ts` contains the `Spec` interface, defined by our CUE `spec` field
* `types.status.gen.ts` contains the `Status` interface, defined by our CUE `status` field

Additional `types.x.gen.ts` files will be generated for each subresource in your schema (and will be added as a field in `Foo`).

### `definitions`

The `definitions` directory holds a JSON (or YAML, depending on CLI flags) Custom Resource Definition (CRD) file for each of your kinds. These files can be applied to a kubernetes API server to generate CRDs for your kinds, which you can then use the other generated code to interface with. For more about CRDs see [Kubernetes Concepts](../kubernetes.md).

## Complex Schemas

### Optional Fields

To mark a field as optional, like in TypeScript, we use a `?` before the `:`. This results in it not being listed as `required` in the OpenAPI specification used for the CRD, and the field type in go uses a pointer. For example:
```cue
{
    foo?: string
}
```
generates
```go
type Spec struct {
	Foo *string `json:"foo,omitempty"`
}
```
and
```typescript
export interface Spec {
  foo?: string;
}
```

### Subtypes

Often your schemas won't be as simple as the example we wrote, and will need sub-types. You can declare these as inline structs in CUE like
```cue
{
    foo: string
    bar: {
        foobar: string
    }
}
```
But you'll end up with go code that isn't very easy to use:
```go
type Spec struct {
    Foo string `json:"foo"`
    Bar struct{
        Foobar string `json:"foobar"`
    } `json:"bar"`
}
```
To generate go types which are more usable, you'll want to embed [CUE definitions](https://cuelang.org/docs/tour/types/defs/). This is simpler than it sounds: all you need to do is define a field that begins with a `#`. This is a definition, and won't be rendered as a field in the generated go, but you can use it as a type, and it will be turned into a go struct with that type name. Here's our example above adjusted to use a CUE definition:
```cue
{
    #Bar: {
        foobar: string
    }
    foo: string
    bar: #Bar
}
```
Now we get more usable go code:
```go
type Spec struct {
	Bar SpecBar `json:"bar"`
	Foo string  `json:"foo"`
}

// SpecBar defines model for spec.#Bar.
type SpecBar struct {
	Foobar string `json:"foobar"`
}
```

A definition can be defined anywhere in the schema, so you could define several definitions outside of `spec` and still use them within `spec` or any other subresource.

### Time types

You can import go types, such as `time.Time` using `import` at the top of your CUE file. However, for codegen to properly handle `time.Time`, you need to union it with `string`, like so:
```cue
package kinds

import "time"

foo: {
    kind: "Foo"
    group: "myapp"
    apiResource: {}
    currentVersion: "v1"
    versions: {
        "v1": {
            schema: {
                spec: {
                    timeField: string & time.Time
                }
            }
        }
    }
}
```

### Constraints

[Bounds](https://cuelang.org/docs/tour/types/bounds/) can be added to your types, such as numerical bounds, or non-nil checks. These will only apply to the generated OpenAPI spec for your CRD, and will not be checked in your go or TypeScript types themselves (or in the generated Codecs). As such, the validation of the bounds is only checked on admission by the kubernetes API (via the apiextensions server that manages CRDs).

You can define further, more complex validation and admission control via your operator using admission webhooks, see [Admission Control](../admission-control.md).

### Examples

Example complex schemas used for codegen testing can be found in the [cuekind codegen testing directory](../../codegen/cuekind/testing/).

## Recommended Reading

* [Managing Multiple Kind Versions](./managing-multiple-versions.md)
* [CUE Documentation](https://cuelang.org/docs/)
* [CUETorials](https://cuetorials.com/)