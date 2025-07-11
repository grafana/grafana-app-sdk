# App Manifest

Every app platform app needs a manifest, which describes all the information about your app needed for the platform to run it. 
This data is primarily split into three groups:
* Your app's supported API Versions (including [kinds](./custom-kinds/README.md) for those versions)
* Your app's capabilities
* The extra permissions your app requires to run

The manifest is a kubernetes object, and a complete one looks something like this:
```yaml
apiVersion: apps.grafana.com/v1alpha1
kind: AppManifest
metadata:
  name: issue-tracker-project
spec:
  appName: example
  group: example.ext.grafana.com
  versions:
    - name: v1alpha1
      served: true
      kinds:
        - kind: Issue
          plural: Issues
          scope: Namespaced
          admission:
            validation:
              operations:
                - CREATE
                - UPDATE
            mutation:
              operations:
                - CREATE
                - UPDATE
          schema:
            spec:
              properties:
                firstField:
                  type: string
              required:
                - firstField
              type: object
          conversion: false
  extraPermissions:
    accessKinds:
      - group: playlist.grafana.app
        resource: playlists
        actions:
          - get
          - list
          - watch
  preferredVersion: "v1alpha1"
```
This manifest describes an app which is called `example` (each appName must be unique), with the API group `example.ext.grafana.com`. 
This app has a kind it manages called `Foo`, which is has the capability to **validate** and **mutate**. 
Finally, it requires permissions to get, list, and watch `playlists` from the `playlist.grafana.app` group.

Typically, you do not need to write your app's manifest yourself, it can instead be generated using the `grafana-app-sdk` CLI.

## Generating a Manifest

By default, your manifest is generated alongside code from CUE. In your project, you'll need a `manifest` CUE object. 
If you use `grafana-app-sdk project init` to set up your project, this is automatically generated for you in `kinds/manifest.cue`. 
A standard `manifest` CUE object looks like:
```cue
package kinds

manifest: {
	// appName is the unique name of your app. It is used to reference the app from other config objects,
	// and to generate the group used by your app in the app platform API.
	appName: "example-app"
	// versions is a map of versions supported by your app. Version names should follow the format "v<integer>" or
	// "v<integer>(alpha|beta)<integer>". Each version contains the kinds your app manages for that version.
	// If your app needs access to kinds managed by another app, use permissions.accessKinds to allow your app access.
	versions: {
	    "v1alpha1": v1alpha1
	    "v2alpha1": v2alpha1
	}
	// extraPermissions contains any additional permissions your app may require to function.
	// Your app will always have all permissions for each kind it manages (the items defined in 'kinds').
	extraPermissions: {
		// If your app needs access to additional kinds supplied by other apps, you can list them here
		accessKinds: [
			// Here is an example for your app accessing the playlist kind for reads and watch
			{
				group: "playlist.grafana.app"
				resource: "playlists"
				actions: ["get","list","watch"]
			}
		]
	}
}

// v1alpha1 is the v1alpha1 version of the app's API.
// It includes kinds which the v1alpha1 API serves
v1alpha1: {
    // kinds is the list of kinds served by this version
    kinds: [mykindv1alpha1]
}

v2alpha1: {
    // kinds is the list of kinds served by this version
    kinds: [mykindv2alpha1, otherkindv2alpha1]
}
```
This manifest exposes two versions: `v1alpha1` and `v2alpha1`. Version `v1alpha1` and `v2alpha1` both contain versions 
of `mykind`, with each using a different value as their schema may differ. `v2alpha1` also contains `otherkind`, 
which is not available in `v1alpha1` (see [Writing Kinds](./custom-kinds/writing-kinds.md) for more details on 
different values for each version of the kind).

The default preferred API version for clients to use is whichever is highest, but can be manually specified with the manifest's
`preferredVersion` field.
This manifest uses the API group `exampleapp.ext.grafana.com`, as the default group is automatically determined from the app name. 
If you are working from an app written in a much older version of the app-sdk, or otherwise need to change the group, 
you can add a `groupOverride` field with a fully-qualified group name to keep your current group. 
This manifest also requires the same extra permissions for playlists as the YAML example manifest above.

To generate a manifest JSON, simply run:
```shell
grafana-app-sdk generate
```
For yaml, you can use the `--crdencoding` flag:
```shell
grafana-app-sdk generate --crdencoding=yaml
```
A manifest isn't all that useful in most scenarios without at least one kind that your app exposes, so be sure you're familiar with [custom kinds](./custom-kinds/README.md) and [writing custom kinds](./custom-kinds/writing-kinds.md).

## Versions

Versions exposed by a manifest should adhere to the scheme `v([0-9]+)((alpha|beta)[0-9]+)?`. Examples include:
* `v1alpha1`
* `v1`
* `v2beta3`
* `v42`

Versions without an `alpha` or `beta` component are considered **stable** versions, and breaking changes should not be made to their schemas or behaviors.
This means that fields in the schemas should not have types changed, required fields should not be added, and no fields should be removed. 
Additionally, admission behavior (validation and mutation) should not change in such a way that previously-acceptable requests begin to fail
(bugfixes excluded). Stable versions should also be supported for a longer period of time than unstable versions.

Versions that contain `alpha` or `beta` are **unstable** versions, and are subject to breaking changes and dropped support. 
Generally, an `alpha` version may have changes that break a client, and is not guaranteed to be long-lived. 
A `beta` version should be more stable than an `alpha` one, and should have a lifetime of several months at minimum.

When both stable and unstable versions are present in a manifest, it's good practice to set the `preferredVersion` 
to the latest **stable** version, so that tooling automatically uses that and will not hit breaking changes, 
but consumers can choose to opt into the newer, unstable versions if they wish to.