# Admission Control

While an app can do some level of admission control by restricting access to the API server to be exclusively through its own plugin API, 
the best practice is to implement admission control at the API server level itself. This is done by exposing webhooks from the 
multi-tenant operator to validate and/or mutate requests to the API server.

The `resource` package contains two interfaces used for admission control:
* [ValidatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/resource#ValidatingAdmissionController), which is used to _validate_ incoming requests, returning a yes/no on whether the request should be allowed to proceed, and
* [MutatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/resource#MutatingAdmissionController), which is used to _alter_ an incoming request, returning am altered object which is translated into series of patch operations which will be made by the API server before proceeding

These interfaces can be implemented by an app author, or the "simple" variants ([SimpleValidatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/resource#SimpleValidatingAdmissionController) and [SimpleMutatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/resource#SimpleMutatingAdmissionController) respectively) may be used.

Once you have a validating and/or mutating admission controller defined, you will need to attach it to your operator. Using kubernetes as the API server, 
the `k8s` package presents a `operator.Controller`-implementing webhook server which can be used: [k8s.WebhookServer](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/k8s#WebhookServer). A very simple example of an operator's `main` which runs both a mutating and validating webhook follows:

```go
func main() {
    // ...setup of the client generator goes here...

    // Valiating admission controller
    validatingController := resource.SimpleValidatingAdmissionController{
        ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
            // Check that the name is allowed
            if request.Object.GetName() == "not-allowed" {
                return k8s.NewSimpleAdmissionError(fmt.Errorf("name not allowed"), http.StatusBadRequest, "ERR_NAME_NOT_ALLOWED")
            }
            return nil
        }
    }

    // Mutating admission controller
    mutatingController := resource.SimpleMutatingAdmissionController{
        MutateFunc func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
            // Add a label
            obj := request.Object
            md := obj.ObjectMetadata()
            if md.Labels == nil {
                md.Labels = make(map[string]string)
            }
            md.Labels["foo"] = "bar"
            obj.SetCommonMetadata(md)
            return &resource.MutatingResponse{
                UpdatedObject: obj,
            }, nil
        }
    }

    webhookController, err := k8s.NewWebhookServer(k8s.WebhookServerConfig{
        Port: 443,
        TLSConfig: loadedTLSConfig,
        ValidatingControllers: map[resource.Schema]resource.ValidatingAdmissionController{
            mykind.Schema(): validatingController,
        },
        MutatingControllers: map[resource.Schema]resource.MutatingAdmissionController{
            mykind.Schema(): mutatingController,
        },
    })

    op := operator.New()

    // ...add other controllers

    op.AddController(webhookController)

    // Run the operator
}
```

## Opinionated Controllers

Much like the `operator` package has the opinionated watcher and reconciler to handle some of the boilerplate work for you, the `k8s` package 
contains opinionated validating and mutating controllers that deal with the non-kubernetes standard metadata in a kind 
(such as `updateTimestamp`, `createdBy`, `updatedBy`).

[k8s.OpinionatedValidatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/k8s#OpinionatedValidatingAdmissionController) 
validates any changes to the grafana metadata annotations and checks that if any changes are made to them, they are allowed changes 
(such as changing `grafana.com/updatedBy` to the user making the request). If a change is invalid (such as changing `grafana.com/updatedBy` to 
a different user than the origin of the request), the controller rejects the request. The controller optionally allows for further validation 
logic to be called by giving it a `ValidatingAdmissionController` in `Underlying`.

[k8s.OpinionatedMutatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/k8s#OpinionatedMutatingAdmissionController) 
updates the grafana metadata annotations in the object to be valid for the request (such as updating `grafana.com/updateTimestamp` to the current timestamp), 
and adds relevant grafana and sdk labels (such as ``). 
The controller optionally allows for other mutations to be performed prior to these updates by supplying a non-nil `MutatingAdmissionController` 
in `Underlying`.

It is generally best practice to use these controllers even is you do not need to extend with any additional custom logic (especially the 
`OpinionatedMutatingAdmissionController`).

## Registering Webhooks

If you are using `grafana-app-sdk project local generate`, you can set
```yaml
webhooks:
  validating: true # or false if you're only doing mutating hooks
  mutating: true # or false if you're only doing validating hooks
  port: 8443 # or whatever port you're using
```

This will have the generated `dev-bundle.yaml` include kubernetes webhook configs, generate the proper TLS cert bundle, and attach it to your operator image, 
mounted in `/run/secrets/tls`. 

For production use, you can either re-use the configs and secrets created by the local environment (they are self-signed, but do not need a real CA as 
they are only used for communication between the API server and the webhook server), or generate new ones. Keep in mind that every time you generate 
the local environment, the cert bundle is generated (and is unique each time), so don't rely on it being consistent.