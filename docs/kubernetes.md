# Kubernetes Concepts

## Custom Resource Definition
Kubernetes documentation articles:
* https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
* https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/

Custom Resource Definitions are a way of extending the kubernetes API to include arbitrary objects. 
These custom objects exhibit no special behavior on their own, and are just a collection of data that can queried and managed like any other resource 
(the objects include all the standard kubernetes metadata, so they can, for example, have attached labels and queries can filter on them). 
What this means in practice is that you can use the kubernetes API or `kubectl` to create, read, list, update, patch, and delete these new kinds of objects that you create. 
Objects have a defined schema, which allows for storing well-typed data. For example, assume we have a CRD called `GrafanaThing` which has the following schema:
```yaml
properties:
  someString:
    type: string
  someInteger:
    type: integer
```
Kubernetes can now store resources of type `GrafanaThing` which can contain a string and an integer. 
On their own, they do nothing, but exist as the same class as any other kubernetes resource. 
So you could list them with `kubectl get grafanathings`, for example.

Custom Resource Definitions must have a **group** and **version**, which are used to identify them, and are part of the API path to managing them.

### CRDs & grafana-app-sdk

When you author kinds and use `grafana-app-sdk generate` to [generate code](code-generation.md), you'll get a CRD file for each kind as well. 
The CRD file will contain the entire lineage expressed in openAPI format, and can be used to create your kind as a CRD in a kubernetes cluster.
The generated `resource.Schema` is then used to identify the **group**, **version**, and **kind** of your CRD when interacting with kubernetes. 
You can then use a `k8s.ClientRegistry` to generate clients which can translate the CRDs in the cluster into the generated go type. 

The client intermediary is used for two main reasons: 
1. It allows us to introduce efficiencies under the hood transparently to the app authors
2. It ensures that we use the encoding/decoding process in the kind's resource.Codec, rather than a straight JSON marshal/unmarshal.

The client intermediary is not strictly necessary for generated resource.Object implementations, but is still favored because of the above two reasons.

You can still directly interface with the CRD's through kubernetes tooling or APIs as well, the SDK's tooling just makes understanding and updating the object's metadata simpler.

## Operator
Kubernetes documentation articles:
* https://kubernetes.io/docs/concepts/extend-kubernetes/operator/

At its core, an operator is an application that does things based on kubernetes state. As an example within kubernetes already, 
there is an operator which watches the changes to the `Deployment` resources, and creates/updates/deletes the necessary resources (such as `ReplicaSets`) when said changes happen. 
An operator does not need to run within kubernetes, and all the operators in the [Examples](../examples) run as binaries on your own machine.

In the context of this SDK, an operator will watch one or more custom resource kinds and react to changes. 
These can be custom resource kinds that are typically created by application(s) that you control (such as a backend plugin, or some other binary), 
or even resources which your code doesn't manage, but you want to take action based upon changes 
(for example, a grafana Data Source may be represented as a CRD, and you may wish to take some action when a Data Source is changed). 

Within the SDK, there is the `operator.Operator` type, which handles one or more `operator.Controller` objects, which are intended to handle specific resources or groups of resources. An `operator.Controller` can be run on its own, as well, without being a part of an `operator.Operator`.
The built-in `operator.Controller` object is the `operator.InformerController`, which uses a pattern of having `operator.Informer` objects which handle emitting events for add, update, or delete actions on a specific kind, and `operator.ResourceWatcher` objects which are user-defined and react to events emitted by Informers (this is very similar to the kubernetes informer pattern, just with a decoupling between the informer and the actions to take for events). A reconciler-pattern controller is also planned for the very near future. 
For more details, see the [Operator Examples](../examples/operator) or the [Operator Package README](../docs/operators.md).

### Running an Operator

An operator can run anywhere where it can access the kubernetes control plane API. 
Often, you want to run the operator within your cluster for latency and convenience reasons, but this is not required. 
As the operator is just an application, it can be run as a pod in kubernetes, as a docker container on a host, or even as a simple binary on bare metal. 
As long as it has access to the kubernetes API, it will function. 

The only other thing to consider is that the operator must have the proper permissions in kubernetes to do the things it wants to do. 
Each Custom Resource kind has its own permission set, so make sure that it's allowed to read/manage/etc. whatever it needs to for each resource kind. 
