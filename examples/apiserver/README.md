# API Server Example

This example is of an app running as a simple API server. It uses etcd to store data.

#### Start etcd

If you don't already have an etcd instance running on port 2379, start etcd with
```shell
make etcd
```
This will run etcd in a docker container and forward port 2379 on your localhost to it

#### Start the API Server

Start the API server with
```shell
make run
```
Logs will be output to standard out

#### (re-)Generate the Code

If you make changes to the CUE in `kinds`, you can re-generate the code with 
```shell
make generate
```

#### Make Requests

You can make unauthenticated requests with cURL:

List TestKinds:
```shell
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds
```

Create a TestKind named "foo":
```shell
curl -k -X POST -H "content-type: application/json" -d '{"apiVersion":"example.ext.grafana.com/v1alpha1","kind":"TestKind","metadata":{"name":"foo","namespace":"default"},"spec":{"testField":"foo"}}' https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds
```

Get the `/foo` subresource (`GetFoo`):
```shell
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds/foo/foo
```

or the `/bar` subresource (`GetMessage`):
```shell
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds/foo/bar
```

Update status of foo:
```shell
curl -k -X PUT -H "content-type: application/json" -d '{"apiVersion":"example.ext.grafana.com/v1alpha1","kind":"TestKind","metadata":{"name":"foo","namespace":"default","resourceVersion":"<RESOURCEVERSION>"},"status":{"additionalFields":{"foo":"bar"}}}' https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds/foo/status
```
Note that `<RESOURCEVERSION>` must be replaced by the current `metadata.resourceVersion`

Update the contents of `mysubresource` (only allowed when hitting the `/mysubresource` path, just like with `/status`):
```shell
curl -k -X PUT -H "content-type: application/json" -d '{"apiVersion":"example.ext.grafana.com/v1alpha1","kind":"TestKind","metadata":{"name":"foo","namespace":"default","resourceVersion":"<RESOURCEVERSION>"},"mysubresource":{"extraValue":"set"}}' https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds/foo/mysubresource
```
Note that `<RESOURCEVERSION>` must be replaced by the current `metadata.resourceVersion`

Attempt to create a TestKind named "notallowed" (forbidden by validation):
```shell
curl -k -X POST -H "content-type: application/json" -d '{"apiVersion":"example.ext.grafana.com/v1alpha1","kind":"TestKind","metadata":{"name":"notallowed","namespace":"default"},"spec":{"testField":"foo"}}' https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds
```

Select by the selectableField `.spec.testField`:
```shell
curl -k "https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds?fieldSelector=spec.testField=foo"
```

Delete `foo`:
```shell
curl -k -X DELETE https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/testkinds/foo
```

Call custom resource routes:
```shell
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/namespaces/default/foobar
```

```shell
curl -k https://127.0.0.1:6443/apis/example.ext.grafana.com/v1alpha1/foobar
```

Get OpenAPI doc:
```shell
curl -k https://127.0.0.1:6443/openapi/v2
```