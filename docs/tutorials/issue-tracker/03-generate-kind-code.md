# Generating Kind Code

Now that we have our kind and schema defined, we want to generate code from them that we can use. 
To do this, we need to add the kind to our **manifest**. The manifest is information about our app, 
including the versions and kinds it provides. If we don't add our kind to the manifest, it isn't considered part of our app. 

Our project init created a default manifest for us, found in `./kinds/manifest.cue`:
```cue
package kinds

manifest: {
	// appName is the unique name of your app. It is used to reference the app from other config objects,
	// and to generate the group used by your app in the app platform API.
	appName: "issue-tracker-project"
	// groupOverride can be used to specify a non-appName-based API group.
	// By default, an app's API group is LOWER(REPLACE(appName, '-', '')).ext.grafana.com,
	// but there are cases where this needs to be changed.
	// Keep in mind that changing this after an app is deployed can cause problems with clients and/or kind data.
	// groupOverride: foo.ext.grafana.app
	
	// versions is a map of versions supported by your app. Version names should follow the format "v<integer>" or
	// "v<integer>(alpha|beta)<integer>". Each version contains the kinds your app manages for that version.
	// If your app needs access to kinds managed by another app, use permissions.accessKinds to allow your app access.
	versions: {
	    "v1alpha1": v1alpha1
	}
	// extraPermissions contains any additional permissions your app may require to function.
	// Your app will always have all permissions for each kind it manages (the items defined in 'kinds').
	extraPermissions: {
		// If your app needs access to additional kinds supplied by other apps, you can list them here
		accessKinds: [
			// Here is an example for your app accessing the playlist kind for reads and watch
			// {
			//	group: "playlist.grafana.app"
			//	resource: "playlists"
			//	actions: ["get","list","watch"]
			// }
		]
	}
}

// v1alpha1 is the v1alpha1 version of the app's API.
// It includes kinds which the v1alpha1 API serves, and (future) custom routes served globally from the v1alpha1 version.
v1alpha1: {
    // kinds is the list of kinds served by this version
    kinds: []
    // [OPTIONAL]
    // served indicates whether this particular version is served by the API server.
    // served should be set to false before a version is removed from the manifest entirely.
    // served defaults to true if not present.
    served: true
    // [OPTIONAL]
    // Codegen is a trait that tells the grafana-app-sdk, or other code generation tooling, how to process this kind.
    // If not present, default values within the codegen trait are used.
    // If you wish to specify codegen per-version, put this section in the version's object
    // (for example, <no value>v1alpha1) instead.
    codegen: {
        // [OPTIONAL]
        // ts contains TypeScript code generation properties for the kind
        ts: {
            // [OPTIONAL]
            // enabled indicates whether the CLI should generate front-end TypeScript code for the kind.
            // Defaults to true if not present.
            enabled: true
        }
        // [OPTIONAL]
        // go contains go code generation properties for the kind
        go: {
            // [OPTIONAL]
            // enabled indicates whether the CLI should generate back-end go code for the kind.
            // Defaults to true if not present.
            enabled: true
        }
    }
}
```

By default, the `appName` is the final part of our module path. We don't need to worry about `extraPermissions`, as our app doesn't need to work with any other apps' kinds. 
`versions` is pre-populated with a `v1alpha1` version object, which has a `kinds` section. 
However, right now, the `kinds` list is empty. To add our kind to the `v1alpha1` version in the manifest, we just need to add the field name of the `v1alpha1` version of our kind:
```cue
kinds: [issuev1alpha1]
```

> [!TIP]
> If you add a new kind to your project with the command `grafana-app-sdk project kind add <KindName>`, it will automatically add it to the manifest as well

In the future, we'll want to re-generate this code whenever we change anything in our `kinds` directory. The SDK provides a command for this: `grafana-app-sdk generate`, but our project init also gave us a make target which will do the same thing, so you can run either. Here, I'm running the make target:
```shell
make generate
```
This command should output a list of all the files it writes:
```shell
$ make generate
 * Writing file pkg/generated/issue/v1alpha1/constants.go
 * Writing file pkg/generated/issue/v1alpha1/issue_codec_gen.go
 * Writing file pkg/generated/issue/v1alpha1/issue_metadata_gen.go
 * Writing file pkg/generated/issue/v1alpha1/issue_object_gen.go
 * Writing file pkg/generated/issue/v1alpha1/issue_schema_gen.go
 * Writing file pkg/generated/issue/v1alpha1/issue_spec_gen.go
 * Writing file pkg/generated/issue/v1alpha1/issue_status_gen.go
 * Writing file plugin/src/generated/issue/v1alpha1/issue_object_gen.ts
 * Writing file plugin/src/generated/issue/v1alpha1/types.metadata.gen.ts
 * Writing file plugin/src/generated/issue/v1alpha1/types.spec.gen.ts
 * Writing file plugin/src/generated/issue/v1alpha1/types.status.gen.ts
 * Writing file definitions/issue.issuetrackerproject.ext.grafana.com.json
 * Writing file definitions/issue-tracker-project-manifest.json
 * Writing file pkg/generated/manifestdata/issuetrackerproject_manifest.go
```
That's a bunch of files written! Let's tree the directory to understand the structure a bit better.
```shell
$ tree .
.
├── cmd
│   └── operator
├── definitions
│   ├── issue-tracker-project-manifest.json
│   └── issue.issuetrackerproject.ext.grafana.com.json
├── go.mod
├── go.sum
├── kinds
│   ├── cue.mod
│   │   └── module.cue
│   ├── issue.cue
│   └── manifest.cue
├── local
│   ├── additional
│   ├── config.yaml
│   ├── mounted-files
│   │   └── plugin
│   ├── scripts
│   │   ├── cluster.sh
│   │   └── push_image.sh
│   └── Tiltfile
├── Makefile
├── pkg
│   └── generated
│       ├── issue
│       │   └── v1alpha1
│       │       ├── constants.go
│       │       ├── issue_codec_gen.go
│       │       ├── issue_metadata_gen.go
│       │       ├── issue_object_gen.go
│       │       ├── issue_schema_gen.go
│       │       ├── issue_spec_gen.go
│       │       └── issue_status_gen.go
│       ├── manifestdata
│       │   └── issuetrackerproject_manifest.go
└── plugin
    └── src
        └── generated
            └── issue
                └── v1alpha1
                    ├── issue_object_gen.ts
                    ├── types.metadata.gen.ts
                    ├── types.spec.gen.ts
                    └── types.status.gen.ts

21 directories, 24 files
```

All of our generated go code lives in `pkg/generated`, and all the generated TypeScript lives in `plugin/src/generated`. 
We also have some JSON files in `definitions`

Note that we also have generated TypeScript in our previously-empty `plugin` directory. By convention, the Grafana plugin for your project will live in the `plugin` directory, so here we've got some TypeScript generated in `plugin/src/generated` to use when we start working on the front-end of our plugin.

## Generated Go Code

The package with the largest number of files generated by `make generate` is the `pkg/generated/issue` package. 
This is also the package where all of our generated go code lives (even with multiple kinds, all generated go code will live in `pkg/generated`). 

Let's take a closer look at the list of files:
```shell
$ tree pkg/generated
pkg/generated
├── issue
│   └── v1alpha1
│       ├── constants.go
│       ├── issue_codec_gen.go
│       ├── issue_metadata_gen.go
│       ├── issue_object_gen.go
│       ├── issue_schema_gen.go
│       ├── issue_spec_gen.go
│       └── issue_status_gen.go
├── manifestdata
    └── issuetrackerproject_manifest.go

4 directories, 8 files
```

The exported go types from our kind's `v1alpha1` schema definition are `issue_spec_gen.go` and `issue_status_gen.go`. 
`issue_metadata_gen.go` exists for legacy reasons we won't touch on here. You'll note that `issue_status_gen.go` contain types and fields which we didn't define in our schema--that's because of the joined "default" status information. 
If we had defined a `status` or `metadata` in our schema, those fields would _also_ be present in the generated types.

In addition to the types generated from our kind's schema, we have `issue_object_gen.go`, `issue_schema_gen.go`, and `issue_codec_gen.go`. 
`issue_object_gen.go` defines the complete object (with `spec`, `status`, and metadata) in a way that satisfies the `resource.Object` interface, so that it can be used with the SDK. 
Likewise, `issue_schema_gen.go` defines a `resource.Schema` for this specific version of the kind which can be used in your project, 
in addition to a `resource.Kind` for the kind. Finally, `issue_codec_gen.go` contains code for a kubernetes-JSON-bytes<->Issue `Object` codec, 
which is used by the `Kind` for marshaling and unmarshaling our Object when interacting with the API server.

Finally, we have `issuetrackerproject_manifest.go` in the `pkg/generated/manifestdata` directory. This contains an in-code version of our manifest. 
As we'll see a bit later, the manifest is also used in code to communicate app capabilities, so we have this in-code representation, 
as well as an API server one.

## Generated TypeScript Code

```shell
$ tree plugin
plugin
└── src
    └── generated
        └── issue
            └── v1alpha1
                ├── issue_object_gen.ts
                ├── types.metadata.gen.ts
                ├── types.spec.gen.ts
                └── types.status.gen.ts

5 directories, 4 files
```

The generated TypeScript contains an interface built from our schema. 
Similarly to our go code, there are types for `Spec` and `Status` (and a legacy `Metadata` type), 
and an `Object` type which pulls them all together. TypeScript code is only generated for kinds where `frontend: true`.

### Generated Custom Resource Definitions

Finally, we have the custom resource definition file that describes our `issue` kind as a CRD, which lives in `definitions` by default. 
Note that this is a CRD of the kind, not just the schema, so the CRD will contain all schema versions in the kind. 
This can be used to set up kubernetes as our storage layer for our project.

We also have a manifest here, which will be used by the grafana API server in the future to register the app.

```shell
$ tree definitions
definitions
├── issue-tracker-project-manifest.json
└── issue.issuetrackerproject.ext.grafana.com.json

1 directory, 2 files
```

So now we have a bunch of generated code, but we still need a project to actually use it in. 
The SDK gives us some tooling to set up our project with boilerplate code, [so let's do that next](04-boilerplate.md).

### Prev: [Defining Our Kinds & Schemas](02-defining-our-kinds.md)
### Next: [Generating Boilerplate](04-boilerplate.md)