package apiserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana/pkg/apimachinery/apis/common/v0alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/server"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/kube-openapi/pkg/common"
)

var _ APIGroupProvider = &ResourceGroup{}

type Resource struct {
	Kind                  resource.Kind
	GetOpenAPIDefinitions common.GetOpenAPIDefinitions
	Subresources          []SubresourceRoute
	Validator             resource.ValidatingAdmissionController
	Mutator               resource.MutatingAdmissionController
	Reconciler            func(generator resource.ClientGenerator, getter OptionsGetter) (operator.Reconciler, error)
	Converter             ResourceConverter
}

func (r *Resource) AddToScheme(scheme *runtime.Scheme) {
	gv := schema.GroupVersion{
		Group:   r.Kind.Group(),
		Version: r.Kind.Version(),
	}
	scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()), r.Kind.ZeroValue())
	scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()+"List"), r.Kind.ZeroListValue())
	// If there are subresource routes, we need to add the ResourceCallOptions to the scheme for the Connector to work
	if len(r.Subresources) > 0 {
		scheme.AddKnownTypes(gv, &ResourceCallOptions{})
		scheme.AddGeneratedConversionFunc((*url.Values)(nil), (*ResourceCallOptions)(nil), func(a, b interface{}, scope conversion.Scope) error {
			return CovertURLValuesToResourceCallOptions(a.(*url.Values), b.(*ResourceCallOptions), scope)
		})
	}
}

func (r *Resource) RegisterAdmissionPlugin(plugins *admission.Plugins) {
	if r.Validator == nil && r.Mutator == nil {
		return
	}
	gvk := r.Kind.Group() + "/" + r.Kind.Version() + "/" + r.Kind.Kind()
	plugins.Register(gvk+"Admission", func(config io.Reader) (admission.Interface, error) {
		return r, nil
	})
}

func (r *Resource) Handles(o admission.Operation) bool {
	if r.Validator == nil && r.Mutator == nil {
		return false
	}
	return true
}

func (r *Resource) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if r.Validator == nil {
		return nil
	}

	if !r.matchesResouce(a) {
		return nil
	}

	req, err := buildAdmissionRequest(a)
	if err != nil {
		return err
	}

	return r.Validator.Validate(ctx, req)
}

func (r *Resource) Admit(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if r.Mutator == nil {
		return nil
	}

	if !r.matchesResouce(a) {
		return nil
	}

	req, err := buildAdmissionRequest(a)
	if err != nil {
		return err
	}

	res, err := r.Mutator.Mutate(ctx, req)
	if err != nil {
		return err
	}

	if err := copyModifiedObjectToDestination(res.UpdatedObject, req.Object); err != nil {
		return errors.NewInternalError(fmt.Errorf("unable to copy updated object to destination: %w", err))
	}

	return nil
}

func (r *Resource) matchesResouce(a admission.Attributes) bool {
	kindMatch := a.GetKind().Kind == r.Kind.Kind()
	groupMatch := a.GetKind().Group == r.Kind.Group()
	versionMatch := a.GetKind().Version == r.Kind.Version()
	return kindMatch && groupMatch && versionMatch
}

func buildAdmissionRequest(a admission.Attributes) (*resource.AdmissionRequest, error) {
	var (
		userInfo    = resource.AdmissionUserInfo{}
		obj, oldObj resource.Object
		ok          bool
	)
	if a.GetUserInfo() != nil {
		userInfoExtra := make(map[string]any)
		for k, v := range a.GetUserInfo().GetExtra() {
			userInfoExtra[k] = v
		}
		userInfo.Extra = userInfoExtra
		userInfo.Groups = a.GetUserInfo().GetGroups()
		userInfo.UID = a.GetUserInfo().GetUID()
		userInfo.Username = a.GetUserInfo().GetName()
	}

	obj, ok = a.GetObject().(resource.Object)
	if !ok {
		return nil, errors.NewInternalError(fmt.Errorf("new obj is not a valid resource.Object"))
	}

	if a.GetOldObject() != nil {
		oldObj, ok = a.GetOldObject().(resource.Object)
		if !ok {
			return nil, errors.NewInternalError(fmt.Errorf("old object is not a valid resource.Object"))
		}
	}

	return &resource.AdmissionRequest{
		Action:    resource.AdmissionAction(a.GetOperation()),
		Kind:      a.GetKind().Kind,
		Group:     a.GetKind().Group,
		Version:   a.GetKind().Group,
		UserInfo:  userInfo,
		Object:    obj,
		OldObject: oldObj,
	}, nil
}

func copyModifiedObjectToDestination(updatedObj runtime.Object, destination runtime.Object) error {
	u, err := conversion.EnforcePtr(updatedObj)
	if err != nil {
		return fmt.Errorf("unable to enforce updated object pointer: %w", err)
	}
	d, err := conversion.EnforcePtr(destination)
	if err != nil {
		return fmt.Errorf("unable to enforce destination pointer: %w", err)
	}
	d.Set(u)
	return nil
}

type SubresourceRoute struct {
	// Path is the path _past_ the resource identifier
	// {schema.group}/{schema.version}/{schema.plural}[/ns/{ns}]/{path}
	Path        string
	OpenAPISpec common.GetOpenAPIDefinitions
	Handler     AdditionalRouteHandler
}

type AdditionalRouteHandler func(w http.ResponseWriter, r *http.Request, identifier resource.Identifier)

// TODO: should this be different from the k8s.Converter? Using the k8s.Converter means we need extra allocations when working with the runtime.Object apimachinery supplies
type Converter interface {
	k8s.Converter
}

type GenericConverter struct{}

func (GenericConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
	codec := resource.NewJSONCodec()
	into := &resource.UntypedObject{}
	err := codec.Read(bytes.NewReader(obj.Raw), into)
	if err != nil {
		return nil, err
	}
	into.SetGroupVersionKind(schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind))
	buf := bytes.Buffer{}
	err = codec.Write(&buf, into)
	return buf.Bytes(), err
}

type ResourceGroup struct {
	Name      string
	Resources []Resource
}

func NewResourceGroup(name string, resources []Resource) *ResourceGroup {
	g := &ResourceGroup{
		Name:      name,
		Resources: resources,
	}
	return g
}

func (g *ResourceGroup) AddToScheme(scheme *runtime.Scheme) error {
	// TODO: this assumes items in the Resources slice are ordered by version
	versions := make(map[metav1.GroupKind][]Resource)
	for _, r := range g.Resources {
		gk := metav1.GroupKind{Group: r.Kind.Group(), Kind: r.Kind.Kind()}
		list, ok := versions[gk]
		if !ok {
			list = make([]Resource, 0)
		}
		list = append(list, r)
		versions[gk] = list
	}

	for gk, vers := range versions {
		// Create an internal version which is set as the latest version in the list for each distinct GroupKind
		latest := vers[len(vers)-1]
		gv := schema.GroupVersion{
			Group:   latest.Kind.Group(),
			Version: runtime.APIVersionInternal,
		}

		scheme.AddKnownTypeWithName(gv.WithKind(gk.Kind), latest.Kind.ZeroValue())
		scheme.AddKnownTypeWithName(gv.WithKind(gk.Kind+"List"), latest.Kind.ZeroListValue())

		// Register each added version with the scheme
		priorities := make([]schema.GroupVersion, len(vers))
		for i, v := range vers {
			groupVersion := schema.GroupVersion{
				Group:   v.Kind.Group(),
				Version: v.Kind.Version(),
			}
			v.AddToScheme(scheme)
			metav1.AddToGroupVersion(scheme, groupVersion)
			priorities[len(priorities)-1-i] = groupVersion

			if v.Kind.Version() == latest.Kind.Version() {
				continue
			}
			converter := v.Converter
			if converter == nil {
				converter = &GenericObjectConverter{GVK: schema.GroupVersionKind{
					Group:   v.Kind.Group(),
					Version: v.Kind.Version(),
					Kind:    v.Kind.Kind(),
				}}
			}

			scheme.AddConversionFunc(v.Kind.ZeroValue(), latest.Kind.ZeroValue(), func(a, b interface{}, scope conversion.Scope) error {
				return converter.ToInternal(a, b)
			})
			scheme.AddConversionFunc(latest.Kind.ZeroValue(), v.Kind.ZeroValue(), func(a, b interface{}, scope conversion.Scope) error {
				return converter.FromInternal(a, b)
			})
		}

		// Set version priorities based on the reverse-order list we built
		scheme.SetVersionPriority(priorities...)
	}
	return nil
}

type StorageProviderFunc func(resource.Kind, *runtime.Scheme, generic.RESTOptionsGetter) (rest.Storage, error)

type StandardStorage interface {
	rest.StandardStorage
	GetSubresources() map[string]SubresourceStorage
}

type SubresourceStorage interface {
	rest.Storage
	rest.Patcher
}

func (g *ResourceGroup) APIGroupInfo(storageProvider StorageProvider, options APIGroupInfoOptions) (*server.APIGroupInfo, error) {
	scheme := options.Scheme
	if scheme == nil {
		scheme = runtime.NewScheme()
		g.AddToScheme(scheme)
	}
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(g.Name, scheme, options.ParameterCodec, options.Codecs)
	for _, r := range g.Resources {
		plural := strings.ToLower(r.Kind.Plural())
		s, err := storageProvider.StandardStorage(r.Kind, scheme)
		if err != nil {
			return nil, err
		}
		store, ok := apiGroupInfo.VersionedResourcesStorageMap[r.Kind.Version()]
		if !ok {
			store = map[string]rest.Storage{}
		}
		// Resource storage
		store[plural] = s
		// Subresource storage
		for k, subRoute := range s.GetSubresources() {
			store[fmt.Sprintf("%s/%s", plural, k)] = subRoute
		}

		// Custom subresource routes
		resourceCaller := &SubresourceConnector{
			Routes: r.Subresources,
		}
		for _, subRoute := range r.Subresources {
			store[fmt.Sprintf("%s/%s", plural, subRoute.Path)] = resourceCaller
		}
		apiGroupInfo.VersionedResourcesStorageMap[r.Kind.Version()] = store
	}
	return &apiGroupInfo, nil
}

func (g *ResourceGroup) RegisterAdmissionPlugins(plugins *admission.Plugins) {
	for i := 0; i < len(g.Resources); i++ {
		g.Resources[i].RegisterAdmissionPlugin(plugins)
	}
}

func (g *ResourceGroup) GetPostStartRunners(generator resource.ClientGenerator, getter OptionsGetter) ([]Runner, error) {
	c := operator.NewInformerController(operator.DefaultInformerControllerConfig()) // TODO: get config from OptionsGetter?
	reconcilerCount := 0
	for _, r := range g.Resources {
		if r.Reconciler != nil {
			kindStr := fmt.Sprintf("%s.%s/%s", r.Kind.Plural(), r.Kind.Group(), r.Kind.Version())
			reconciler, err := r.Reconciler(generator, getter)
			if err != nil {
				return nil, fmt.Errorf("error creating reconciler for %s: %w", kindStr, err)
			}
			err = c.AddReconciler(reconciler, kindStr)
			if err != nil {
				return nil, fmt.Errorf("error adding reconciler for %s: %w", kindStr, err)
			}
			client, err := generator.ClientFor(r.Kind)
			if err != nil {
				return nil, fmt.Errorf("error creating kubernetes client for %s: %w", kindStr, err)
			}
			informer, err := operator.NewKubernetesBasedInformer(r.Kind, client, resource.NamespaceAll)
			if err != nil {
				return nil, fmt.Errorf("error creating informer for %s: %w", kindStr, err)
			}
			err = c.AddInformer(informer, kindStr)
			if err != nil {
				return nil, fmt.Errorf("error adding informer for %s to controller: %w", kindStr, err)
			}
			reconcilerCount++
		}
	}
	if reconcilerCount == 0 {
		return nil, nil
	}
	return []Runner{c}, nil
}

func schemeConversionFunc(r1, r2 resource.Kind, converter k8s.Converter) func(any, any, conversion.Scope) error {
	// TODO: This has extra allocations, do we want converters to be object -> object rather than bytes -> bytes?
	return func(a, b interface{}, scope conversion.Scope) error {
		fromObj, ok := a.(resource.Object)
		if !ok {
			return fmt.Errorf("from type is not a valid resource.Object")
		}
		fromBytes := &bytes.Buffer{}
		err := r1.Write(fromObj, fromBytes, resource.KindEncodingJSON)
		if err != nil {
			return err
		}
		toObj, ok := b.(resource.Object)
		if !ok {
			return fmt.Errorf("to type is not a valid resource.Object")
		}
		converted, err := converter.Convert(k8s.RawKind{
			Kind:       r1.Kind(),
			APIVersion: fmt.Sprintf("%s/%s", r1.Group(), r1.Version()),
			Group:      r1.Group(),
			Version:    r1.Version(),
			Raw:        fromBytes.Bytes(),
		}, fmt.Sprintf("%s/%s", r2.Group(), r2.Version()))
		if err != nil {
			return err
		}
		return r2.Codec(resource.KindEncodingJSON).Read(bytes.NewReader(converted), toObj)
	}
}

// GetOpenAPIDefinitions combines the provided list of getters and standard grafana and kubernetes OpenAPIDefinitions
// into a single GetOpenAPIDefinitions function which can be used with a kubernetes API Server.
func GetOpenAPIDefinitions(getters []common.GetOpenAPIDefinitions) common.GetOpenAPIDefinitions {
	return func(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		// TODO: extract v0alpha1 openAPI into app-sdk, or leave in grafana?
		defs := v0alpha1.GetOpenAPIDefinitions(ref) // common grafana apis
		for _, fn := range getters {
			out := fn(ref)
			maps.Copy(defs, out)
		}
		maps.Copy(defs, GetResourceCallOptionsOpenAPIDefinition())
		return defs
	}
}
