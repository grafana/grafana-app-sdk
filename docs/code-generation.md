# Code Generation

Code generation is done via the CLI, with 
```
grafana-app-sdk generate [-c|--cuepath=kinds] [-g|--gogenpath=pkg/generated] [-t|--tsgenpath=plugin/src/generated]
```
Code is generated from [Custom Kinds](./custom-kinds.md), and either gives you code that is designed to work with the `resource` package, 
or base types with multi-version marshal/unmarshal capabilities.

Currently, the switch between these types depends on the presence of `crd` in the kind (`crd` being present creates a `resource`-package-compatible set of files, its absense generates more simple files with just the marshal/unmarshal). 
This may change in the future to be based on package or initial namespace in CUE.

The codegen generates two types of code and possibly accessory files (such as Custom Resource Definitions). 
Where the code lives, and what kind of code is generated can be controlled via CLI flags and the kind's `codegen` trait respectively.

`-g` or `--gogenpath` governs where the generated go code is placed. By default this is `pkg/generated`. 
The next portion of the path is always set to `(resource|models)/<LOWER(kind name)>/`.

`-t` ot `--tsgenpath` governs where the generated TypeScript code is placed. This defaults to `plugin/src/generated`, 
and all generated TS files will be in that folder.

Whether go code, TypeScript code, or both is generated is governed by the `codegen` trait in your kind:
```cue
myKind: {
    name: "Foo"
    group: "foo-app"
    codegen: {
        frontend: true // this defaults to true and dictates whether TypeScript code is generated
        backend: true // this defaults to true and dictates whether go code is generated
    }
    lineage: {...}
}
```
Since these have defaults, you can leave `codegen` out entirely if you are ok with the default values, or only specify the fields you want to change.

## `resource`-package code (`crd` present)

For code compatible with the `resource` package interfaces, you get a few main pieces of exported code. 
All go files generated will be under `pkg/generated/resource/<your kind name>/`, with a package of `LOWER(kind name)`. 
Types generated:
* `Object` - this is the type which implements `resource.Object`. It contains your spec, status, metadata, and static metadata.
* `Schema()` - this function returns a `resource.Schema` implementation for your kind.
* `Spec` - the struct for the spec component of your lineage's latest schema
* `Status` - the struct for the status component of your lineage's latest schema (with kindsys status info joined in)
* `Metadata` - the struct for the metadata component of your lineage's latest schema (with kindsys metadata info joined in)

Some interesting bits to note about the implementation within the go codegen:
* The kind's lineage is extracted and re-written in your generated files so thema can bind it at runtime
* Currently, the `Unmarshal` method in `Object` only handles a `JSON` wire format

TypeScript interfaces will be generated for each kind that adhere to the latest schema as-written in the lineage. 
To use these against an API server, a translation layer must be in-place like what exists in the SDK go library. 
Implementation of this layer in TypeScript is currently forthcoming. 
Currently, the best method is to expose a backend API via the plugin that accepts the TypeScript-interface-style 
(or similar--the generated `Object` struct is nearly identical to the kind schema format with the addition of the `StaticMeta` field which adds identifier information), 
and have the back-end plugin do the reads and writes from the storage layer.

As the only valid argument for the flag `--type` is currently `kubernetes`, Custom Resource Definition files will also be generated in `--crdpath` (defaults to `definitions`). 
The format of the file can be governed by `--crdencoding` (valid values of `json` or `yaml`, defaults to `json`). 

The default value of `--type` is subject to change in the future as other storage layer options become available based on what is decided to be the stand use-case for the SDK codegen.

## Simple Model Code

The code generation for non-`resource` package code (sometimes referred to as `model` code) generates fewer exported types. 
The generated files are located in `pkg/generated/models/<your kind name>/`, with a package of `LOWER(kind name)`.
Types generated:
* `<kind name>` - A go struct with the latest schema in your kind's lineage
* `Marshal()` - a function to marshal the type using a specific version
* `Unmarshal()` - a function to unmarshal bytes into the type (from any schema version)

The generated typescript code for a model is identical to the resource type codegen.

## Examples & Testing

Code generation is done as part of the [issue tracker tutorial](./tutorials/issue-tracker/03-generate-schema-code.md).

Automated testing of code generation from CUE kinds is done using the files in [codegen/testing/cue](../codegen/testing/cue/), with generated files compared against [codegen/testing/golden_generated](../codegen/testing/golden_generated/).