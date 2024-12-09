# App Manifest

Every app platform app needs a manifest, which describes all the information about your app needed for the platform to run it. 
This data is primarily split into three groups:
* Your app's [kinds](./custom-kinds/README.md)
* Your app's capabilities
* The extra permissions your app requires to run

The manifest is a kubernetes object, and a complete one looks something like this:
```yaml
apiVersion: apps.grafana.com/v1
kind: AppManifest
metadata:
  name: example
spec:
  appName: example
  group: example.ext.grafana.com
  kinds:
    - kind: Foo
      scope: Namespaced
      versions:
        - name: v1
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
	// kinds is the list of kinds that your app defines and manages. If your app deals with kinds defined/managed
	// by another app, use permissions.accessKinds to allow your app access
	kinds: [myKind1, myKind2]
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
```
This manifest has two kinds (the kinds defined by the CUE selectors `myKind1` and `myKind2`, see [Writing Kinds](./custom-kinds/writing-kinds.md)), 
and uses the API group `exampleapp.ext.grafana.com`. The group is automatically determined from the app name. 
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