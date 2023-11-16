# Custom Kinds

Custom kinds are the base of code generation in the SDK, and are considered the canonical data model for all resource types handled by the SDK. A custom kind definition includes schemas for all available versions, and metadata about the kind itself and codegen directives. The abstract definition of a kind as a go interface for code generation [can be found here](https://github.com/grafana/grafana-app-sdk/blob/06a1baa56f039bce9685f64a5a0594afbe092128/codegen/kind.go#L8), but is not neeeded unless you wish to define you own manner for parsing kinds for code generation.

Custom kinds can be defined multiple ways, but the preferred method is using CUE, [using the Kind definition defined in codegen/cuekind/def.cue](https://github.com/grafana/grafana-app-sdk/blob/main/codegen/cuekind/def.cue). Other formats for defining kinds are available with the `-f` flag in the CLI.

# CUE Kind

If you have an existing project, you can create a template for a new kind using the CLI with
```
grafana-app-sdk project kind add MyKindName
```
This will provide a kind template with all optional fields and comments on what each field means and what it does.

A kind is composed of two main components: data about the kind ("kind metadata"), and a set of versioned schemas. In CUE, the minimum description of an example kind would be:
```cue
{
    kind: "MyKindName"
    group: "my-app"
    apiResource: {} // This tells the codegen that this kind is a kind which can be expressed as a kubernetes API server resource
    current: "v1" // This defines what the current version is of our kind. Even if we only have one version (like here), it is still required
    versions: {
        "v1": {
            schema: {
                spec: {
                    // spec must contain at least one field, of any supported type
                    atLeastOneFieldName: string
                }
            }
        }
    }
}
```

### Recommended Reading

* [CUE Documentation](https://cuelang.org/docs/)
* [CUETorials](https://cuetorials.com/)
