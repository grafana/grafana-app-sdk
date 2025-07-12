# Managing Multiple Kind Versions

There are two kinds of versions for a kind: a fixed, **stable** version, such as `v1`, and a mutable, **unstable**, alpha or beta version, such as `v1alpha1`. 
When publishing a kind, be aware that you should never change the definition (in a breaking way) of a stable version, as clients will rely on that kind version being constant when working with the API. 
For more details on kubernetes version conventions, see [Custom Resource Definitions Versions](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/).

## Reconcilers & Multiple Versions

No matter how many versions you have, you should only have one watcher/reconciler per kind, as resource events are not local to a version. When you create an informer, you are specifying a specific version you want to consume _all_ events for the kind as, and your converter hook will be called to convert the object in each event into the requested version (if necessary). Typically, this means that you want your watcher/reconciler to consume the latest version, as it will usually contain the most information.

## Adding a New Version

Versions of a kind are managed in your app's manifest, with a list of versioned kinds existing for each version. 
It is best practice to have an object in CUE for each version of your kind, which share a "kind metadata" object that contains 
information such as the kind name, plural, scope, etc.

If you already have this set up (this is the default setup when adding a kind with `grafana-app-sdk project kind add`), 
adding a new version is just adding a new object, and adding a reference to that object in your manifest's `versions` object.
```cue
fooKind: {
	kind: "Foo"
	// other existing kind information
}

// existing v1 version
foov1: fooKind & {
	schema: {
		// existing v1 schema
	}
}

// new v2 version
foov2: fooKind & {
	schema: {
		// new v2 schema
	}
}
```

Then adding that version to your manifest as well:
```cue
manifest: {
	// existing manifest data
	versions: {
		"v1": v1 // existing v1
		"v2": v2 // new v2
	}
}

// existing v1 version
v1: {
	kinds: [foov1]
}

// new v2 version
v2: {
	kinds: [foov2]
}
```

However, now that you have two versions, you'll need to be able to support both of them, since any user can request either version from the API server. 
By default, the API server will convert between these versions by simply taking the JSON schema from one and pushing it into the other (depending on what is stored), 
but often that is not a good enough conversion, and you'll need to define how to convert between versions yourself.

To do this, you'll need to add **Conversion** capability to your app. This must be done both in your app's manifest (where you'll indicate that conversion is supported for the kind), 
and in your app code (where the logic to handle the conversion actually lives). 

## Manifest

To add conversion to your manifest, you just need to specify in the kind that you support custom conversion logic with the `conversion` boolean in your kind metadata object, like so: 
```cue
fooKind: {
	// Existing kind information
	conversion: true
}
```
If you run your app as a standalone operator (currently the only runner in the grafana-app-sdk, `operator.Runner`), 
conversion gets exposed as a webhook. In the future, this will be handled by registering the app manifest, but for now, 
since CRDs are generated and applied separately, you'll need to specify the URL of the webhook to put into the CRD's 
conversion information when it's generated:
```cue
{
	// Existing kind information
	conversion: true
	conversionWebhookProps: url: "https://myapp.svc.cluster.local:6443/convert"
}
```
(by default, the operator runner will expose the conversion webhook on the `/convert` endpoint. 
You can specify the port of the webhook server in `operator.RunnerConfig.WebhookConfig.Port`).

## Conversion Code

For the conversion code, your app needs to have code that runs as part of the call to the app's `Convert` function. 
If you're implementing `app.App` yourself, you would add your conversion handling there
(keep in mind that the function is called for _any_ kinds your app supports when the `/convert` endpoint is hit for them, 
so you'll need to make sure you handle each one). 
If you're using `simple.App`, to expose conversion behavior for a kind, you need to add it to the config:
```go
app, err := simple.NewApp(simple.AppConfig{
	Converters: map[schema.GroupKind]simple.Converter{
		schema.GroupKind{Group: v1.Kind().Group, Kind: v1.Kind().Kind}: &MyKindConverter{},
	}
})
```
> [!NOTE]  
> This structure for registering converters (and the `simple.Converter` interface itself) is still in flux and likely to change to a more ergonomic one in the future. 
> See [this issue](https://github.com/grafana/grafana-app-sdk/issues/617) for tracking.

The `Converters` map takes a `schema.GroupKind` as its key, which uniquely identifies a kind, and the interface `simple.Converter` as the value. 
So, to implement conversion, let's take a look at `simple.Converter`. Right now, this just aliases to `k8s.Converter`, which is defined as:
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

`k8s.RawKind` contains the raw (JSON) bytes of an object, and kind information (Group, Version, Kind). To implement `k8s.Converter` we need a function which can accept any version of our kind and return any version of our kind. When we register our converter in `simple.App`, we can generally assume that the input 
`RawKind` will be of the Group and Kind we specify, and the output will also be of the Group and Kind we specify, so it's safe to error on anything unexpected. Let's put together a very simple converter for an object with two versions defined as:
```cue
fooKind: {
	kind: "MyKind"
	conversion: true
	
}

// existing v1 version
foov1: fooKind & {
	schema: {
		spec: {
			foo: string
		}
	}
}

// new v2 version
foov2: fooKind & {
	schema: {
		spec: {
			foo: string
			bar: string
		}
	}
}

manifest: {
	appName: "myApp"
	versions: {
		"v1": {
			kinds: [foov1]
		}
		"v2": {
			kinds: [foov2]
		}
	}
	operatorURL: "https://myapp.svc.cluster.local:6443"
}
```
```go
type MyKindConverter struct {}

func (m *MyKindConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
    // We shouldn't ever see this, but just in case...
    if targetAPIVersion == obj.APIVersion {
        return nil, fmt.Errorf("conversion from a version to itself should not call the webhook: %s", targetAPIVersion)
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

Now, when running our app using `operator.Runner`, we just need to ensure that we specify webhook configuration
(if you are already exposing validation or mutation behavior, you'll have already done this, as it uses the same webhook server):
```go
runner, err := operator.NewRunner(operator.RunnerConfig{
	// Existing configuration
	WebhookConfig: operator.RunnerWebhookConfig{
		Port: 6443,
		TLSConfig: k8s.TLSConfig{
			CertPath: "/path/to/cert",
			KeyPath: "/path/to/key",
		},
	},
})
```
