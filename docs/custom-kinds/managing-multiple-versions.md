# Managing Multiple Kind Versions

There are two kinds of versions for a kind: a fixed version, such as `v1`, and a mutable, alpha or beta version, such as `v1alpha1`. When publishing a kind, be aware that you should never change the definition of a fixed version, as clients will rely on that kind version being constant when working with the API. For more details on kubernetes version conventions, see [Custom Resource Definitions Versions](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/).

## Operators & Multiple Versions

No matter how many versions you have, you should only have one watcher/reconciler per kind, as resource events are not local to a version. When you create an informer, you are specifying a specific version you want to consume _all_ events for the kind as, and your converter hook will be called to convert the object in each event into the requested version (if necessary). Typically, this means that you want your watcher/reconciler to consume the latest version, as it will usually contain the most information.

## Adding a New Version

When you need to add a new version to a kind, it's rather simple in the standard CUE definition:
```cue
{
    // Existing kind information
    versions: {
        "v1": { // Current version
            schema: {
                // schema
            } 
        }
        "v2": { // New version
            schema: {
                // new schema
            }
        }
    }
}
```

However, now that you have two versions, you'll need to be able to support both of them, since any user can request either version from the API server. 
By default, the API server will convert between these versions by simply taking the JSON schema from one and pushing it into the other (depending on what is stored), 
but often that is not a good enough conversion, and you'll need to define how to convert between versions yourself.

To do this, you'll need to add a Conversion Webhook to your operator. If you're using the `simple.Operator` type, this can be done either in the initial config with the `simple.OperatorConfig.Webhooks.Converters` field, or via the `ConvertKind` method on `simple.Operator`. If you are not using `simple.Operator`, you'll need to create a `k8s.WebhookServer` controller to add to your operator with `k8s.NewWebhookServer` (you may already have this if you're using [Admission Control](./admission-control.md), as Conversion webhooks are exposed on the same server as Validation and Mutation webhooks). All of these different methods require two things: a `k8s.Converter`-implementing type, and TLS information. Let's go over each, then give some examples.

### Converter

`k8s.Converter` is defined as:
```go
// Converter describes a type which can convert a kubernetes kind from one API version to another.
// Typically there is one converter per-kind, but a single converter can also handle multiple kinds.
type Converter interface {
	// Convert converts a raw kubernetes kind into the target APIVersion.
	// The RawKind argument will contain kind information and the raw kubernetes object,
	// and the returned bytes are expected to be a raw kubernetes object of the same kind and targetAPIVersion
	// APIVersion. The returned kubernetes object MUST have an apiVersion that matches targetAPIVersion.
	Convert(obj RawKind, targetAPIVersion string) ([]byte, error)
}
```
`k8s.RawKind` contains the raw (JSON) bytes of an object, and kind information (Group, Version, Kind). To implement `k8s.Converter` we need a function which can accept any version of our kind and return any version of our kind. When we register `k8s.Converter`s with the `WebhookServer`, we can generally assume that the input 
`RawKind` will be of the Group and Kind we specify, and the output will also be of the Group and Kind we specify, so it's safe to error on anything unexpected. Let's put together a very simple converter for an object with two versions defined as:
```cue
myKind: {
    kind: "MyKind"
    versions: {
        "v1": { 
            schema: {
                foo: string
            } 
        }
        "v2": { 
            schema: {
                foo: string
                bar: string
            }
        }
    }
}
```
```go
type MyKindConverter struct {}

func (m *MyKindConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
    // We shouldn't ever see this, but just in case...
    if targetAPIVersion == obj.APIVersion {
        return nil, obj.Raw
    }
    targetGVK := schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind)

    if obj.Version == "v1" {
        // Only allowed conversion is to v2 (as we already checked v1 => v1 above)
        if targetGVK.Version != "v2" {
            return nil, fmt.Errorf("cannot convert into unknown version %s", targetGVK.Version)
        }
        src := v1.MyKind{}
        err := v1.Kind().Codec(resource.KindEncodingJSON).Read(bytes.NewReader(obj.Raw), &src)
        if err != nil {
            return nil, fmt.Errorf("unable to parse kind")
        }
        dst := v2.MyKind{}
        // Copy metadata
        src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
        // Set GVK
        dst.SetGroupVersionKind(targetGVK)
        // Set values
        dst.Spec.Foo = src.Spec.Foo
        buf := bytes.Buffer{}
        err := v2.Kind().Write(&dst, &buf, resource.KindEncodingJSON)
        return buf.Bytes(), err
    }

    if obj.Version == "v2" {
        // Only allowed conversion is to v1 (as we already checked v2 => v2 above)
        if targetGVK.Version != "v1" {
            return nil, fmt.Errorf("cannot convert into unknown version %s", targetGVK.Version)
        }
        src := v2.MyKind{}
        err := v2.Kind().Codec(resource.KindEncodingJSON).Read(bytes.NewReader(obj.Raw), &src)
        if err != nil {
            return nil, fmt.Errorf("unable to parse kind")
        }
        dst := v1.MyKind{}
        // Copy metadata
        src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
        // Set GVK
        dst.SetGroupVersionKind(targetGVK)
        // Set values
        dst.Spec.Foo = src.Spec.Foo
        buf := bytes.Buffer{}
        err := v1.Kind().Write(&dst, &buf, resource.KindEncodingJSON)
        return buf.Bytes(), err
    }

    return nil, fmt.Errorf("unknown source version %s", obj.Version)
}
```

Now, depending on the way we are creating our operator, we can register the converter webhook:

#### `simple.Operator`

**Using OperatorConfig**
```go
runner, err := simple.NewOperator(simple.OperatorConfig{
    Name:       "my-operator",
	KubeConfig: kubeConfig.RestConfig,
    Webhooks:   simple.WebhookConfig{
        Enabled:   true,
        TLSConfig: k8s.TLSConfig{
            CertPath: "/path/to/cert",
            KeyPath:  "/path/to/key",
        },
        Converters: map[metav1.GroupKind]k8s.Converter{
            metav1.GroupKind{Group: v1.Group(), Kind: v1.Kind()}: &MyKindConverter{},
        },
    },
})
```
**Using ConvertKind**
```go
runner, err := simple.NewOperator(simple.OperatorConfig{
    Name:       "my-operator",
	KubeConfig: kubeConfig.RestConfig,
    Webhooks:   simple.WebhookConfig{ // Webhook information is still required in config
        Enabled:   true,
        TLSConfig: k8s.TLSConfig{
            CertPath: "/path/to/cert",
            KeyPath:  "/path/to/key",
        },
    },
})

err = runner.ConvertKind(metav1.GroupKind{Group: v1.Group(), Kind: v1.Kind()}, &MyKindConverter{})
```

#### `k8s.NewWebhookServer`

**In WebhookServerConfig**
```go
ws, err = k8s.NewWebhookServer(k8s.WebhookServerConfig{
    Port:      8443,
    TLSConfig: k8s.TLSConfig{
        CertPath: "/path/to/cert",
        KeyPath:  "/path/to/key",
    },
    KindConverters: map[metav1.GroupKind]k8s.Converter{
        metav1.GroupKind{Group: v1.Group(), Kind: v1.Kind()}: &MyKindConverter{},
    },
})

err = runner.AddController(ws)
```

**Using AddConverter**
```go
ws, err = k8s.NewWebhookServer(k8s.WebhookServerConfig{
    Port:      8443,
    TLSConfig: k8s.TLSConfig{
        CertPath: "/path/to/cert",
        KeyPath:  "/path/to/key",
    },
})

ws.AddConverter(&MyKindConverter{}, metav1.GroupKind{Group: v1.Group(), Kind: v1.Kind()})

err = runner.AddController(ws)
```

### Using the Converter Webhook

If you use `grafana-app-sdk project local generate`, you can set `converting: true` in the `webhooks` section of `local/config.yaml`. This will set the webhook conversion strategy in your CRD and make sure your operator service exposes your webhook port. However, this only applies for local setup. 

> [!WARNING]  
> To register your conversion webhook in production, you must currently manually update the generated CRD file!

Update your generated CRD file to add the `conversion` block as described in [the kubernetes documentation](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#configure-customresourcedefinition-to-use-conversion-webhooks) before deploying the CRD to take advantage of the converter webhook you have written