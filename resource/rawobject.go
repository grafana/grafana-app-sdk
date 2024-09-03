package resource

import (
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ Object = &RawUntypedObject{}

// RawUntypedObject is an implementation of Object that doesn't parse any data, leaving it as a completely-unstructured
// map[string]any contained by the field Raw. If you would like typed metadata, use UntypedObject instead.
type RawUntypedObject struct {
	// Raw contains an unstructured map of all data in the object
	Raw map[string]any
}

func (r *RawUntypedObject) UnmarshalJSON(data []byte) error {
	r.Raw = make(map[string]any)
	return json.Unmarshal(data, &r.Raw)
}

func (r *RawUntypedObject) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Raw)
}

func (r *RawUntypedObject) GetObjectKind() schema.ObjectKind {
	return r
}

func (r *RawUntypedObject) DeepCopyObject() runtime.Object {
	return r.Copy()
}

func (r *RawUntypedObject) SetGroupVersionKind(kind schema.GroupVersionKind) {
	apiVersion, k := kind.ToAPIVersionAndKind()
	r.Raw["apiVersion"] = apiVersion
	r.Raw["kind"] = k
}

func (r *RawUntypedObject) GroupVersionKind() schema.GroupVersionKind {
	apiVersion := ""
	if a, ok := r.Raw["apiVersion"]; ok {
		apiVersion, _ = a.(string)
	}
	kind := ""
	if k, ok := r.Raw["kind"]; ok {
		kind, _ = k.(string)
	}
	return schema.FromAPIVersionAndKind(apiVersion, kind)
}

func (r *RawUntypedObject) GetNamespace() string {
	return r.getStringMetadataField("namespace")
}

func (r *RawUntypedObject) SetNamespace(namespace string) {
	r.setMetadataField("namespace", namespace)
}

func (r *RawUntypedObject) GetName() string {
	return r.getStringMetadataField("name")
}

func (r *RawUntypedObject) SetName(name string) {
	r.setMetadataField("name", name)
}

func (r *RawUntypedObject) GetGenerateName() string {
	return r.getStringMetadataField("generateName")
}

func (r *RawUntypedObject) SetGenerateName(name string) {
	r.setMetadataField("generateName", name)
}

func (r *RawUntypedObject) GetUID() types.UID {
	return types.UID(r.getStringMetadataField("uid"))
}

func (r *RawUntypedObject) SetUID(uid types.UID) {
	r.setMetadataField("uid", string(uid))
}

func (r *RawUntypedObject) GetResourceVersion() string {
	return r.getStringMetadataField("resourceVersion")
}

func (r *RawUntypedObject) SetResourceVersion(version string) {
	r.setMetadataField("resourceVersion", version)
}

func (r *RawUntypedObject) GetGeneration() int64 {
	gen := r.getAnyMetadataField("generation")
	if gen == nil {
		return 0
	}
	cast, _ := gen.(int64)
	return cast
}

func (r *RawUntypedObject) SetGeneration(generation int64) {
	r.setMetadataField("generation", generation)
}

func (r *RawUntypedObject) GetSelfLink() string {
	return r.getStringMetadataField("selfLink")
}

func (r *RawUntypedObject) SetSelfLink(selfLink string) {
	r.setMetadataField("selfLink", selfLink)
}

func (r *RawUntypedObject) GetCreationTimestamp() metav1.Time {
	raw := r.getAnyMetadataField("creationTimestamp")
	if raw == nil {
		return metav1.Time{}
	}
	if ts, ok := raw.(metav1.Time); ok {
		return ts
	}
	if ts, ok := raw.(time.Time); ok {
		return metav1.NewTime(ts)
	}
	return metav1.Time{}
}

func (r *RawUntypedObject) SetCreationTimestamp(timestamp metav1.Time) {
	r.setMetadataField("creationTimestamp", timestamp)
}

func (r *RawUntypedObject) GetDeletionTimestamp() *metav1.Time {
	raw := r.getAnyMetadataField("deletionTimestamp")
	if raw == nil {
		return nil
	}
	if ts, ok := raw.(*metav1.Time); ok {
		return ts
	}
	if ts, ok := raw.(metav1.Time); ok {
		return &ts
	}
	if ts, ok := raw.(*time.Time); ok && ts != nil {
		t := metav1.NewTime(*ts)
		return &t
	}
	if ts, ok := raw.(time.Time); ok {
		t := metav1.NewTime(ts)
		return &t
	}
	return nil
}

func (r *RawUntypedObject) SetDeletionTimestamp(timestamp *metav1.Time) {
	r.setMetadataField("deletionTimestamp", timestamp)
}

func (r *RawUntypedObject) GetDeletionGracePeriodSeconds() *int64 {
	raw := r.getAnyMetadataField("deletionGracePeriodSeconds")
	if raw == nil {
		return nil
	}
	if ts, ok := raw.(*int64); ok {
		return ts
	}
	if ts, ok := raw.(int64); ok {
		return &ts
	}
	return nil
}

func (r *RawUntypedObject) SetDeletionGracePeriodSeconds(i *int64) {
	r.setMetadataField("deletionGracePeriodSeconds", i)
}

func (r *RawUntypedObject) GetLabels() map[string]string {
	raw := r.getAnyMetadataField("labels")
	if raw == nil {
		return make(map[string]string)
	}
	if cast, ok := raw.(map[string]string); ok {
		return cast
	}
	return make(map[string]string)
}

func (r *RawUntypedObject) SetLabels(labels map[string]string) {
	r.setMetadataField("labels", labels)
}

func (r *RawUntypedObject) GetAnnotations() map[string]string {
	raw := r.getAnyMetadataField("annotations")
	if raw == nil {
		return make(map[string]string)
	}
	if cast, ok := raw.(map[string]string); ok {
		return cast
	}
	return make(map[string]string)
}

func (r *RawUntypedObject) SetAnnotations(annotations map[string]string) {
	r.setMetadataField("annotations", annotations)
}

func (r *RawUntypedObject) GetFinalizers() []string {
	raw := r.getAnyMetadataField("finalizers")
	if raw == nil {
		return make([]string, 0)
	}
	if cast, ok := raw.([]string); ok {
		return cast
	}
	return make([]string, 0)
}

func (r *RawUntypedObject) SetFinalizers(finalizers []string) {
	r.setMetadataField("finalizers", finalizers)
}

func (r *RawUntypedObject) GetOwnerReferences() []metav1.OwnerReference {
	raw := r.getAnyMetadataField("ownerReferences")
	if raw == nil {
		return make([]metav1.OwnerReference, 0)
	}
	if cast, ok := raw.([]metav1.OwnerReference); ok {
		return cast
	}
	// Check if the data is of []OwnerReference, even if the type isn't
	if cast, ok := raw.([]map[string]any); ok {
		refs := make([]metav1.OwnerReference, 0)
		for _, v := range cast {
			or := metav1.OwnerReference{}
			or.APIVersion, _ = v["apiVersion"].(string)
			or.Kind, _ = v["kind"].(string)
			or.Name, _ = v["name"].(string)
			uid, _ := v["uid"].(string)
			or.UID = types.UID(uid)
			if c, exists := v["controller"]; exists {
				if con, ok := c.(bool); ok {
					or.Controller = &con
				}
			}
			if b, exists := v["blockOwnerDeletion"]; exists {
				if bod, ok := b.(bool); ok {
					or.BlockOwnerDeletion = &bod
				}
			}
			refs = append(refs, or)
		}
		return refs
	}
	return make([]metav1.OwnerReference, 0)
}

func (r *RawUntypedObject) SetOwnerReferences(references []metav1.OwnerReference) {
	r.setMetadataField("ownerReferences", references)
}

func (r *RawUntypedObject) GetManagedFields() []metav1.ManagedFieldsEntry {
	raw := r.getAnyMetadataField("managedFields")
	if raw == nil {
		return make([]metav1.ManagedFieldsEntry, 0)
	}
	if cast, ok := raw.([]metav1.ManagedFieldsEntry); ok {
		return cast
	}
	// Check if the data is of []ManagedFieldsEntry, even if the type isn't
	if cast, ok := raw.([]map[string]any); ok {
		entries := make([]metav1.ManagedFieldsEntry, 0)
		for _, v := range cast {
			mfe := metav1.ManagedFieldsEntry{}
			mfe.Manager, _ = v["manager"].(string)
			operation, _ := v["operation"].(string)
			mfe.Operation = metav1.ManagedFieldsOperationType(operation)
			mfe.APIVersion, _ = v["apiVersion"].(string)
			if t, exists := v["time"]; exists {
				if cast, ok := t.(metav1.Time); ok {
					mfe.Time = &cast
				}
				if cast, ok := t.(time.Time); ok {
					tm := metav1.NewTime(cast)
					mfe.Time = &tm
				}
			}
			mfe.FieldsType, _ = v["fieldsType"].(string)
			if fv1, exists := v["fieldsV1"]; exists {
				if cast, ok := fv1.(metav1.FieldsV1); ok {
					mfe.FieldsV1 = &cast
				}
				if cast, ok := fv1.(json.RawMessage); ok {
					f := metav1.FieldsV1{
						Raw: cast,
					}
					mfe.FieldsV1 = &f
				}
				if cast, ok := fv1.([]byte); ok {
					f := metav1.FieldsV1{
						Raw: cast,
					}
					mfe.FieldsV1 = &f
				}
			}
			mfe.Subresource, _ = v["subresource"].(string)
			entries = append(entries, mfe)
		}
		return entries
	}
	return make([]metav1.ManagedFieldsEntry, 0)
}

func (r *RawUntypedObject) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
	r.setMetadataField("managedFields", managedFields)
}

func (r *RawUntypedObject) GetSpec() any {
	return r.Raw["spec"]
}

func (r *RawUntypedObject) SetSpec(a any) error {
	r.Raw["spec"] = a
	return nil
}

func (r *RawUntypedObject) GetSubresources() map[string]any {
	subresources := make(map[string]any)
	for k, v := range r.Raw {
		if k == "status" || k == "metadata" || k == "apiVersion" || k == "kind" {
			continue
		}
		subresources[k] = v
	}
	return subresources
}

func (r *RawUntypedObject) GetSubresource(s string) (any, bool) {
	sr, ok := r.Raw[s]
	return sr, ok
}

func (r *RawUntypedObject) SetSubresource(key string, val any) error {
	r.Raw[key] = val
	return nil
}

func (r *RawUntypedObject) GetStaticMetadata() StaticMetadata {
	gvk := r.GroupVersionKind()
	return StaticMetadata{
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
		Namespace: r.GetNamespace(),
		Name:      r.GetName(),
	}
}

func (r *RawUntypedObject) SetStaticMetadata(metadata StaticMetadata) {
	r.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   metadata.Group,
		Version: metadata.Version,
		Kind:    metadata.Kind,
	})
	r.SetNamespace(metadata.Namespace)
	r.SetName(metadata.Name)
}

func (r *RawUntypedObject) GetCommonMetadata() CommonMetadata {
	cmd := CommonMetadata{
		UID:               string(r.GetUID()),
		ResourceVersion:   r.GetResourceVersion(),
		Generation:        r.GetGeneration(),
		Labels:            r.GetLabels(),
		CreationTimestamp: r.GetCreationTimestamp().Time,
		Finalizers:        r.GetFinalizers(),
	}
	if dts := r.GetDeletionTimestamp(); dts != nil {
		deref := *dts
		ts := deref.Time
		cmd.DeletionTimestamp = &ts
	}
	annotations := r.GetAnnotations()
	if annotations != nil {
		if updt, ok := annotations[AnnotationUpdateTimestamp]; ok && updt != "" {
			ts, err := time.Parse(time.RFC3339, updt)
			if err == nil {
				cmd.UpdateTimestamp = ts
			}
		}
		cmd.UpdatedBy = annotations[AnnotationUpdatedBy]
		cmd.CreatedBy = annotations[AnnotationCreatedBy]
	}
	return cmd
}

func (r *RawUntypedObject) SetCommonMetadata(metadata CommonMetadata) {
	r.SetUID(types.UID(metadata.UID))
	r.SetResourceVersion(metadata.ResourceVersion)
	r.SetGeneration(metadata.Generation)
	r.SetLabels(metadata.Labels)
	r.SetCreationTimestamp(metav1.NewTime(metadata.CreationTimestamp))
	if metadata.DeletionTimestamp != nil {
		ts := metav1.NewTime(*metadata.DeletionTimestamp)
		r.SetDeletionTimestamp(&ts)
	} else {
		r.SetDeletionTimestamp(nil)
	}
	r.SetFinalizers(metadata.Finalizers)
	annotations := r.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[AnnotationCreatedBy] = metadata.CreatedBy
	annotations[AnnotationUpdatedBy] = metadata.UpdatedBy
	annotations[AnnotationUpdateTimestamp] = metadata.UpdateTimestamp.Format(time.RFC3339)
}

func (r *RawUntypedObject) Copy() Object {
	// TODO: something better here
	m, _ := json.Marshal(r.Raw)
	dst := make(map[string]interface{})
	_ = json.Unmarshal(m, &dst)
	return &RawUntypedObject{
		Raw: dst,
	}
}

func (r *RawUntypedObject) getMetadata() map[string]any {
	if m, ok := r.Raw["metadata"]; ok {
		if cast, ok := m.(map[string]any); ok {
			return cast
		}
	}
	return map[string]any{}
}

func (r *RawUntypedObject) setMetadata(meta map[string]any) {
	r.Raw["metadata"] = meta
}

func (r *RawUntypedObject) getStringMetadataField(name string) string {
	if field, ok := r.getMetadata()[name]; ok {
		str, _ := field.(string)
		return str
	}
	return ""
}

func (r *RawUntypedObject) getAnyMetadataField(name string) any {
	if field, ok := r.getMetadata()[name]; ok {
		return field
	}
	return nil
}

func (r *RawUntypedObject) setMetadataField(name string, value any) {
	md := r.getMetadata()
	md[name] = value
	r.setMetadata(md)
}

func copyMap(src, dst map[string]any) {
	for k, v := range src {
		switch t := v.(type) {
		case map[string]any:
			entry := make(map[string]any)
			copyMap(t, entry)
			dst[k] = entry
		default:
			// TODO: better
			dst[k] = v
		}
	}
}
