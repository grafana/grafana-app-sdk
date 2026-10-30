# Admission Control

Admission control is validation and/or mutation logic which runs synchronously when a resource of one of your kinds is created or updated,
before the object is stored. Because it is invoked by the platform rather than by your UI, it applies no matter how the request was made
(your plugin UI, `kubectl`, gitops tooling, or another app).

There are two kinds of admission control:
* **Validation**, which returns a yes/no on whether the request should be allowed to proceed (the returned error is shown to the user), and
* **Mutation**, which returns an altered object (for example, with defaults or labels added) to be stored instead of the one in the request.

Admission hooks block the request, so keep them fast and free of side effects. Any slow work, or work that talks to other systems, belongs in a [reconciler](./writing-a-reconciler.md).

## Adding Admission Control to a Kind

The easiest way to add admission control is through `simple.App`, by setting `Validator` and/or `Mutator` on an `AppManagedKind`.
`simple.Validator` and `simple.Mutator` are ready-to-use implementations which call the function you provide:

```go
func NewApp(cfg app.Config) (app.App, error) {
    return simple.NewApp(simple.AppConfig{
        Name:       "my-app",
        KubeConfig: cfg.KubeConfig,
        ManagedKinds: []simple.AppManagedKind{{
            Kind: mykind.Kind(),
            Validator: &simple.Validator{
                ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
                    // Check that the name is allowed
                    if request.Object.GetName() == "not-allowed" {
                        return k8s.NewAdmissionError(fmt.Errorf("name not allowed"), http.StatusBadRequest, "ERR_NAME_NOT_ALLOWED")
                    }
                    return nil
                },
            },
            Mutator: &simple.Mutator{
                MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
                    // Add a label
                    obj := request.Object
                    labels := obj.GetLabels()
                    if labels == nil {
                        labels = make(map[string]string)
                    }
                    labels["foo"] = "bar"
                    obj.SetLabels(labels)
                    return &app.MutatingResponse{
                        UpdatedObject: obj,
                    }, nil
                },
            },
        }},
    })
}
```

You also need to declare validation and/or mutation for the kind in your [manifest](./app-manifest.md), so the platform knows to call your app.
`simple.App.ValidateManifest` will return an error if the capabilities in your code and your manifest don't match.
For a step-by-step walkthrough, see [part 7 of the tutorial](./tutorials/issue-tracker/07-adding-admission-control.md).

If you implement `app.App` yourself, put your logic in its `Validate` and `Mutate` methods instead.

## Grafana Metadata

The `k8s` package also contains opinionated validating and mutating controllers which keep the grafana-specific metadata on an object
(such as `updateTimestamp`, `createdBy`, and `updatedBy`) consistent with the request:
[k8s.OpinionatedValidatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/k8s#OpinionatedValidatingAdmissionController) and
[k8s.OpinionatedMutatingAdmissionController](https://pkg.go.dev/github.com/grafana/grafana-app-sdk/k8s#OpinionatedMutatingAdmissionController).
Each can wrap your own validation or mutation logic via `Underlying`.

## Local Development

If you are using `grafana-app-sdk project local generate`, you can set
```yaml
webhooks:
  validating: true # or false if you're only doing mutating hooks
  mutating: true # or false if you're only doing validating hooks
  port: 8443 # or whatever port you're using
```
in `local/config.yaml` to have your local environment call your operator for admission.
