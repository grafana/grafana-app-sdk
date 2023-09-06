# Custom Kinds

Custom kinds are the base of code generation in the SDK, and are considered the canonical data model for all resource types handled by the SDK. 
Custom kinds are defined in CUE.

The CUE definition of a custom kind lives in [kindsys](https://github.com/grafana/grafana-app-sdk/kindsys/blob/ebfbbc0e58bf49a00a658341f3286ba5fecc056d/kindcat_custom.cue#L106). When writing a custom kind, you do not need to import kindsys (or thema), as they are implicitly imported as part of the code generation process. 
Instead, you only need to define values which have no deafults (and you are free to define values where you want to diverge from the defaults).

If you have an existing project, you can create a template for a new kind using the CLI with
```
grafana-app-sdk project kind add MyKindName
```

A kind is composed of two main components: data about the kind, and the sequence of versioned schemas known as a [thema lineage](https://github.com/grafana/thema). The minimum set of a custom kind used for code generation is:
```cue
{
    name: "MyKindName"
    group: "my-app"
    crd: {}
    lineage: {
        schemas: [{
            version: [0,0]
            schema: {
                spec: {
                    // spec must contain at least one field, of any supported type
                    atLeastOneFieldName: string
                }
            }
        }]
    }
}
```

### Recommended Reading

* [About Thema](https://github.com/grafana/thema/blob/main/docs/overview.md)
* [CUE Documentation](https://cuelang.org/docs/)
* [CUETorials](https://cuetorials.com/)
