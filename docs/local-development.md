# Local Development & Testing

The SDK CLI provides the `project local` command for local development, which by default uses [K3D](https://k3d.io) and [Tilt](https://tilt.dev). These tools can be substituted for other compatible ones, but you may need to tinker with automated functionality, or parts of the automated setup may not work.

## Setup

If you set up your project with

```
grafana-app-sdk project init
```

then you should already have a `local` directory in the root of your project. If you did not, you can run

```
grafana-app-sdk project local init
```

in the root of your project to create the `local` directory and its files.

In the `local` directory, we have:

```shell
$ tree local
local
├── Tiltfile
├── additional
├── config.yaml
├── mounted-files
│   └── plugin
└── scripts
    ├── cluster.sh
    └── push_image.sh

5 directories, 4 files
```

Let's break down what each of these files is for:
| File | Purpose |
|------|---------|
| `Tiltfile` | Tiltfile written in [Starlark](https://github.com/bazelbuild/starlark). This configures Tilt. |
| `additional/` | Directory containing user-generated and source-controlled kubernetes YAML files to apply alongside the generated ones. |
| `config.yaml` | Configuration file for the `grafana-app-sdk` CLI to use when generating kubernetes manifests and the K3D config. |
| `mounted-files/` | Everything in here gets mounted in the K3D cluster. `mounted-files/plugin` is where the built plugin should be placed to be properly mounted in the grafana instance. |
| `scripts/cluster.sh` | K3D cluster control scripts. |
| `scripts/push_image.sh` | Script to push an image from a local registry to the K3D internal registry. |

If you created your project with `project init`, you'll also have some default `Makefile` targets. If you didn't (and for clarity in this doc), here's the Makefile snippit:

```Makefile
.PHONY: local/up
local/up: local/generate
	@sh local/scripts/cluster.sh create "local/generated/k3d-config.json"
	@cd local && tilt up

.PHONY: local/generate
local/generate:
	@grafana-app-sdk project local generate

.PHONY: local/down
local/down:
	@cd local && tilt down

.PHONY: local/deploy_plugin
local/deploy_plugin:
	-tilt disable grafana
	cp -R plugin/dist local/mounted-files/plugin/dist
	-tilt enable grafana

.PHONY: local/push_operator
local/push_operator:
	# Tag the docker image as part of localhost, which is what the generated k8s uses to avoid confusion with the real operator image
	@docker tag "$(OPERATOR_DOCKERIMAGE):latest" "localhost/$(OPERATOR_DOCKERIMAGE):latest"
	@sh local/scripts/push_image.sh "localhost/$(OPERATOR_DOCKERIMAGE):latest"

.PHONY: local/clean
local/clean: local/down
	@sh local/scripts/cluster.sh delete
```

(`OPERATOR_DOCKERIMAGE` is defined at the top of your Makefile)

## `local/config.yaml`

Before we get into generating our local environment or breaking down the steps, let's take a look at our `local/config.yaml`:

```yaml
# Port used to bind services to localhost, for example, grafana will be available at http://grafana.k3d.localhost:9999
port: 9999
# Port used for the kubernetes APIServer on localhost
kubePort: 8556
# Pre-configured datasources to install on grafana. For custom-configuration datasources, use `datasourceConfigs`
datasources:
  - cortex
# Plugin JSON data, as key/value pairs
pluginJson:
  foo: bar
# Plugin Secure JSON data. By default, the `kubeconfig` and `kubenamespace` values will be added in the generated YAML.
# You can overwrite those values by specifying them here instead.
pluginSecureJson:
  baz: foo
# Standalone operator docker image. Leave this empty to not deploy an operator
operatorImage: "foo:latest"
# Non-standard or additional datasources you want to automatically include in grafana's provisioned list
# The actual datasources need to be set up manually (arbitrary kubernetes yamls can be added to the local setup via the 'additional' folder),
# but you can predefine the connection details so they'll be added to the local grafana.
datasourceConfigs:
# Here is an example cortex config
#  - access: proxy
#    editable: false
#    name: "my-cortex-datasource"
#    type: prometheus
#    uid: "my-cortex-datasource"
#    url: "http://cortex.default.svc.cluster.local:9009/api/prom"
#
# Toggle the generating of the grafana deployments, if you want to control these elsewhere
generateGrafanaDeployment: true

# Install plugins from other sources (URLS). See https://grafana.com/docs/grafana/latest/setup-grafana/configure-docker#install-plugins-from-other-sources
grafanaInstallPlugins: ""
```

Most of these fields are broken down in the comments, but let's quickly touch on a few things:

- `port`: this defaults to 9999, and is the port you use on your localhost to get to containers running in the cluster which expose a web interface (such as grafana). If you already have a process that runs on 9999 on your localhost, make sure to change this.
- `kubePort`: this port is also bound on your localhost, and is the port which you will use to talk to the kubernetes API server. Again, if somethings else is already bound to this port on your localhost, make sure to change this.
- `datasources`: Pre-configred datasources to set up in your local environment. Right now, only `cortex` is supported. You can check the list of supported datasources in [ cmd/grafana-app-sdk/project_local_datasources.go](../cmd/grafana-app-sdk/project_local_datasources.go).
- `operatorImage`: make sure the image sans-tag (`:latest`) matches the image name you're building your operator with. If you used the SDK to initialize your project, this should automatically match.

OK, with that out of the way, let's discuss doing a local deployment.

## Local Deployment

A local deployment consists of three steps:

1. Generate the kubernetes manifests and local kubernetes config
2. Start up the kubernetes cluster
3. Deploy the cluster resources

In the default Makefile, these three steps are all done with the `make local/up` command. This command runs the following:

```
$ grafana-app-sdk project local generate
```

This generates the kube manifests and k3d config file based on your `local/config.yaml`. The generated manifests and config are placed in `local/generated`.
_Generally, this directory should not be commited to source control_, as the k3d config requires an absolute path for your mounted volumes, which is user-specific. Additionally, local deployments should be re-generating the kubernetes manifests every time.

```
$ sh local/scripts/cluster.sh create "local/generated/k3d-config.json"
```

This creates the k3d kubernetes cluster using the generated k3d config. If the cluster is already running, this is a no-op.

```
$ cd local && tilt up
```

Finally, we run tilt from the `local` directory. The `Tiltfile` there gathers all the kubernetes objects from `local/generated`, and then goes through all YAML files in `local/additional`, and if any kubernetes objects conflict (same name and kind), the ones in `local/additional` are used instead. This allows you to manually overwrite generated manifests as needed without needing to commit the generated code or worry about your changes being overwritten after a new generate command. It also allows you to add arbitrary kubernetes manifests to your local deployment in addition to what's created by the generate command.

This is all well and good, but for a full local deployment, we need two additional things:

- the plugin built and deployed to `local/mounted-files/plugin`, and
- the operator image built and pushed to the k3d registry

For the plugin, this can be done prior to setting up the local deployment if desired, or after it. If it is done after, keep in mind that the `grafana` container will be in a crash loop until the plugin is deployed. With the default makefile, you can deploy (or redeploy if you've made changes) the plugin with

```
make local/deploy_plugin
```

By hand, you can do it this way:

```
cp -R plugin/dist local/mounted-files/plugin/dist
```

If you _already_ have the plugin deployed and are redploying a new version, you'll need to disable the grafana deployment first, as you can't overwrite the backend binary while it's in-use (the make target does this automatically).

For the operator image, this must be done _after_ the cluster is up, as there's no way to push an image to the k3d registry prior to that. Again, the default Makefile has a target for this:

```
make local/push_operator
```

To do this step manually, you'll need to first make sure your locally-built image is tagged with the `localhost/` prefix in the image name, as that's what the generated operator kubernetes deployment uses (the reason for this is twofold: to avoid confusion with production operator images of the same name, and to create seamless compatibility with docker or podman as tooling). If it isn't already called `localhost/<image name>:latest`, tag it with:

```
docker tag "<image name>:latest" "localhost/<image name>:latest
```

With that done, you can push it to the k3d registry with:

```
sh local/scripts/push_image.sh "localhost/<image name>:latest"
```

The `local/scripts/push_image.sh` script will work with any image you provide it as well, so if you need additional local images pushed to your local deployment for manifests in `local/additional`, this can be used for that, too.

With those extra two steps done, you should now have a working local deployment.

## Accessing Your Deployment

Once up, your local grafana can be accessed via [grafana.k3d.localhost:9999](http://grafana.k3d.localhost:9999) (if you used a `port` other than 9999 in your `local/config.yaml`, use that instead in the URL). 
The default username and password for the local grafana is `admin`/`admin`. 
You can enable anonymous auth using a `Viewer` role by setting `grafanaWithAnonymousAuth: true` in `local/config.yaml`.

If you use kubectl, your kubeconfig should have its default context changed to the local cluster, so you can get any resources that way.

You can access logs for all your deployments via the Tilt UI as well.

## Making Changes

Any changes you make are automatically picked up by Tilt while your local deployment is running. If you make changes to the plugin, just re-deploy the plugin (either with the make target or manually), and they should be present. For operator changes, push the new operator image, and restart the operator deployment (you can do this from the Tilt UI) to get the changes deployed.
