# Opinionated Example

The operator SDK includes an "opinionated" watcher that makes some decisions on resource handling for you. This comes at a very small performance cost to handling these things yourself in the watcher (see `/example/operator/boilerplate` for how this can be done), but allows for your code to not have to care about the logic of handling synchronization, missed events, no-op updates, or blocking deletes when the operator isn't running.

## To Run

### Using the run script
1. Make sure you have k3d and go1.18+ installed
2. Run the following to create a local k3d cluster and start the operator:
    ```shell
   $ ./run.sh
    ```

### Manually
Start a local kubernetes cluster or use a remote one to which you have permission to create CRD's and monitor them. 
Set your kube context to the appropriate cluster, then run the operator:
```shell
$ go run opinionated.go --kubecfg="path_to_your_kube_config"
```


You may see one each of an error message about failing to watch and list the custom resource, 
this is due to a slight delay between the custom resource being added, and the control plane knowing it exists. 
This will only happen on the first run, when the resource definition doesn't yet exist in your cluster.

## Usage

The operator will monitor for changes to any OpinionatedCustomResource in your cluster, and log them. To demonstrate, run:
```shell
$ kubectl create -f example.yaml
```
You should see a log line for the added resource.

You can delete the custom resource and see the delete event with
```shell
$ kubectl delete OpinionatedCustomResource test-resource
```

To demonstrate the "opinionated" watcher capabilities, try adding the example custom resource, then restarting the operator. 
Note that the log line is now a `SYNC` rather than an `ADD`. The watcher knows that this resource has already been added during a previous run, 
and only may have been updated during the restart time. Also note that updates to the resource that do not affect any spec 
or significant parts of the metadata (such as adding a label) will not trigger an `UPDATE`. If you use a custom resource with a `status` subresource (see `crd.BaseStatus`), 
updating the `status` subresource will not trigger an `UPDATE` (this is particularly useful if you use the `status` 
to maintain a record of operations that your operator has performed, and don't want to get into recursive update loops).

To demonstrate that deletes are not missed, you can stop the operator and delete your `OpinionatedCustomResource`. 
Note that the `kubectl delete` will hang until you restart the operator, when you will see a `DELETE` event in the operator logs, 
and the `kubectl` command will complete. The opinionated watcher uses a finalizer attached to the resource to make sure that deletes only complete 
after it has acknowledged them. Note that once the resource has received the `delete` request, even though it still exists in the cluster, it cannot be updated. 
At this point, kubernetes is simply waiting for all the finalizers on the object to be removed before it hard deletes it.

## Managing Custom Resources

You can see the custom resource created by the operator (and all custom resources in your cluster) with
```shell
$ kubectl get CustomResourceDefinitions
```
If you want to remove the custom resource definition created by the operator, you can do so with
```shell
$ kubectl delete CustomResourceDefinition opinionatedcustomresources.example.grafana.com
```
Note that if there are any OpinionatedCustomResources in your cluster, they will be deleted. 
If the operator is not currently running, this command will hang while it waits for the resource deletion 
(you can either run the operator, or remove the finalizer from each resource yourself via a kubectl patch). 
If the operator is running, you will begin seeing errors in the console output, as the list/watch requests to kubernetes will now result in errors.