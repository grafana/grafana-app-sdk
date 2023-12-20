package k8s

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/grafana/grafana-app-sdk/resource"
)

type k8sObject struct {
	metav1.TypeMeta `json:",inline"`
	ObjectMetadata  metav1.ObjectMeta `json:"metadata"`
	Spec            json.RawMessage   `json:"spec"`
	Status          json.RawMessage   `json:"status"`
	Scale           json.RawMessage   `json:"scale"`
}

type k8sListWithItems struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ListMeta   `json:"metadata"`
	Items           []json.RawMessage `json:"items"`
}

// this is janky
//
// nolint:funlen
func rawToObject(raw []byte, into resource.Object) error {
	if into == nil {
		return fmt.Errorf("into cannot be nil")
	}
	// Parse the bytes into parts we can handle (spec, status, scale, metadata)
	kubeObject := k8sObject{}
	err := json.Unmarshal(raw, &kubeObject)
	if err != nil {
		return err
	}

	// Unmarshal the spec and subresources into the object
	subresources := make(map[string][]byte)
	if kubeObject.Status != nil && len(kubeObject.Status) > 0 {
		subresources["status"] = kubeObject.Status
	}
	if kubeObject.Scale != nil && len(kubeObject.Scale) > 0 {
		subresources["scale"] = kubeObject.Scale
	}
	// Metadata from annotations
	meta := make(map[string]any)
	for k, v := range kubeObject.ObjectMetadata.Annotations {
		if len(k) > len(AnnotationPrefix) && k[:len(AnnotationPrefix)] == AnnotationPrefix {
			meta[k[len(AnnotationPrefix):]] = v
		}
	}
	// All the (required) CommonMetadata fields--thema parse gets mad otherwise
	// TODO: overhaul this whole thing so we can just set everything in the metadata without a two-step process
	if _, ok := meta[annotationUpdateTimestamp]; !ok {
		meta[annotationUpdateTimestamp] = kubeObject.ObjectMetadata.CreationTimestamp.Format(time.RFC3339Nano)
	}
	if _, ok := meta[annotationCreatedBy]; !ok {
		meta[annotationCreatedBy] = ""
	}
	if _, ok := meta[annotationUpdatedBy]; !ok {
		meta[annotationUpdatedBy] = ""
	}
	meta["resourceVersion"] = kubeObject.ObjectMetadata.ResourceVersion
	meta["generation"] = kubeObject.ObjectMetadata.Generation
	meta["uid"] = kubeObject.ObjectMetadata.UID
	meta["creationTimestamp"] = kubeObject.ObjectMetadata.CreationTimestamp

	amd, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	err = into.Unmarshal(resource.ObjectBytes{
		Spec:         kubeObject.Spec,
		Subresources: subresources,
		Metadata:     amd,
	}, resource.UnmarshalConfig{
		WireFormat:  resource.WireFormatJSON,
		VersionHint: kubeObject.ObjectMetadata.Labels[versionLabel],
	})
	if err != nil {
		return err
	}

	// Set the static metadata
	into.SetStaticMetadata(resource.StaticMetadata{
		Namespace: kubeObject.ObjectMetadata.Namespace,
		Name:      kubeObject.ObjectMetadata.Name,
		Group:     kubeObject.TypeMeta.GroupVersionKind().Group,
		Version:   kubeObject.TypeMeta.GroupVersionKind().Version,
		Kind:      kubeObject.TypeMeta.Kind,
	})

	// Set the object metadata
	cmd := into.CommonMetadata()
	cmd.ResourceVersion = kubeObject.ObjectMetadata.ResourceVersion
	cmd.Generation = kubeObject.ObjectMetadata.Generation
	cmd.Labels = kubeObject.ObjectMetadata.Labels
	cmd.UID = string(kubeObject.ObjectMetadata.UID)
	if cmd.ExtraFields == nil {
		cmd.ExtraFields = make(map[string]any)
	}
	cmd.Finalizers = kubeObject.ObjectMetadata.Finalizers
	if len(kubeObject.ObjectMetadata.OwnerReferences) > 0 {
		cmd.ExtraFields["ownerReferences"] = kubeObject.ObjectMetadata.OwnerReferences
	}
	if len(kubeObject.ObjectMetadata.ManagedFields) > 0 {
		cmd.ExtraFields["managedFields"] = kubeObject.ObjectMetadata.ManagedFields
	}
	if len(kubeObject.ObjectMetadata.Annotations) > 0 {
		cmd.ExtraFields["annotations"] = kubeObject.ObjectMetadata.Annotations
	}
	cmd.CreationTimestamp = kubeObject.ObjectMetadata.CreationTimestamp.Time
	if kubeObject.ObjectMetadata.DeletionTimestamp != nil {
		t := kubeObject.ObjectMetadata.DeletionTimestamp.Time
		cmd.DeletionTimestamp = &t
	}

	into.SetCommonMetadata(cmd)

	return nil
}

func rawToListWithParser(raw []byte, into resource.ListObject, itemParser func([]byte) (resource.Object, error)) error {
	um := k8sListWithItems{}
	err := json.Unmarshal(raw, &um)
	if err != nil {
		return err
	}
	// Attempt to parse all items in the list before setting the parsed metadata,
	// as the parser could return an error, and we don't want to _partially_ unmarshal the list (just metadata) into `into`
	items := make([]resource.Object, 0)
	for _, item := range um.Items {
		parsed, err := itemParser(item)
		if err != nil {
			return err
		}
		items = append(items, parsed)
	}
	into.SetListMetadata(resource.ListMetadata{
		ResourceVersion:    um.Metadata.ResourceVersion,
		Continue:           um.Metadata.Continue,
		RemainingItemCount: um.Metadata.RemainingItemCount,
	})
	into.SetItems(items)
	return nil
}

type convertedObject struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            any               `json:"spec"`
	Status          any               `json:"status,omitempty"`
	Scale           any               `json:"scale,omitempty"`
}

func marshalJSON(obj resource.Object, extraLabels map[string]string, cfg ClientConfig) ([]byte, error) {
	if obj == nil {
		return nil, fmt.Errorf("obj cannot be nil")
	}

	co := convertedObject{
		TypeMeta: metav1.TypeMeta{
			Kind: obj.StaticMetadata().Kind,
			APIVersion: schema.GroupVersion{
				Group:   obj.StaticMetadata().Group,
				Version: obj.StaticMetadata().Version,
			}.Identifier(),
		},
		Metadata: getV1ObjectMeta(obj, cfg),
		Spec:     obj.SpecObject(),
	}
	if co.Metadata.Labels == nil {
		co.Metadata.Labels = make(map[string]string)
	}
	for k, v := range extraLabels {
		co.Metadata.Labels[k] = v
	}

	// Status and Scale subresources, if applicable
	if status, ok := obj.Subresources()[string(resource.SubresourceStatus)]; ok {
		co.Status = status
	}
	if scale, ok := obj.Subresources()[string(resource.SubresourceScale)]; ok {
		co.Scale = scale
	}

	return json.Marshal(co)
}

var metaV1Fields = getV1ObjectMetaFields()

func marshalJSONPatch(patch resource.PatchRequest) ([]byte, error) {
	// Correct for differing metadata paths in kubernetes
	for idx, op := range patch.Operations {
		// We don't allow a patch on the metadata object as a whole
		if op.Path == "/metadata" {
			return nil, fmt.Errorf("cannot patch entire metadata object")
		}

		// We only need to (possibly) correct patch operations for the metadata
		if len(op.Path) < len("/metadata/") || op.Path[:len("/metadata/")] != "/metadata/" {
			continue
		}
		// If the next part of the path isn't a key in metav1.ObjectMeta, then we put it in annotations
		parts := strings.Split(strings.Trim(op.Path, "/"), "/")
		if len(parts) <= 1 {
			return nil, fmt.Errorf("invalid patch path")
		}
		if _, ok := metaV1Fields[parts[1]]; ok {
			// Normal kube metadata
			continue
		}
		// UNLESS it's extraFields, which holds implementation-specific extra fields
		if parts[1] == "extraFields" {
			if len(parts) < 3 {
				return nil, fmt.Errorf("cannot patch entire extraFields, please patch fields in extraFields instead")
			}
			// Just take the remaining part of the path and put it after /metadata
			// If it's not a valid kubernetes metadata object, that's because the user has done something funny with extraFields,
			// and it wouldn't be properly encoded/decoded by the translator anyway
			op.Path = "/metadata/" + strings.Join(parts[2:], "/")
		} else {
			// Otherwise, update the path to be in annotations, as that's where all the custom and non-kubernetes common metadata goes
			// We just have to prefic the remaining part of the path with the AnnotationPrefix
			// And replace '/' with '~1' for encoding into a patch path
			endPart := strings.Join(parts[1:], "~1") // If there were slashes, we need to encode them
			op.Path = fmt.Sprintf("/metadata/annotations/%s%s", strings.ReplaceAll(AnnotationPrefix, "/", "~1"), endPart)
			if op.Operation == resource.PatchOpReplace {
				op.Operation = resource.PatchOpAdd // We change this for safety--they behave the same within a map, but if they key is absent, replace won't work
			}
		}
		patch.Operations[idx] = op
	}
	return json.Marshal(patch.Operations)
}

func getV1ObjectMetaFields() map[string]struct{} {
	fields := make(map[string]struct{})
	typ := reflect.TypeOf(metav1.ObjectMeta{})
	for i := 0; i < typ.NumField(); i++ {
		jsonTag := typ.Field(i).Tag.Get("json")
		if len(jsonTag) == 0 || jsonTag[0] == '-' || jsonTag[0] == ',' {
			continue
		}
		if idx := strings.Index(jsonTag, ","); idx > 0 {
			jsonTag = jsonTag[:idx]
		}
		fields[jsonTag] = struct{}{}
	}
	return fields
}

func getV1ObjectMeta(obj resource.Object, cfg ClientConfig) metav1.ObjectMeta {
	cMeta := obj.CommonMetadata()
	meta := metav1.ObjectMeta{
		Name:              obj.StaticMetadata().Name,
		Namespace:         obj.StaticMetadata().Namespace,
		UID:               types.UID(cMeta.UID),
		ResourceVersion:   cMeta.ResourceVersion,
		Generation:        cMeta.Generation,
		CreationTimestamp: metav1.NewTime(cMeta.CreationTimestamp),
		Labels:            cMeta.Labels,
		Finalizers:        cMeta.Finalizers,
		Annotations:       make(map[string]string),
	}
	// Rest of the metadata in ExtraFields
	for k, v := range cMeta.ExtraFields {
		switch strings.ToLower(k) {
		case "ownerReferences":
			if o, ok := v.([]metav1.OwnerReference); ok {
				meta.OwnerReferences = o
			}
		case "managedFields":
			if m, ok := v.([]metav1.ManagedFieldsEntry); ok {
				meta.ManagedFields = m
			}
		case "annotations":
			if a, ok := v.(map[string]string); ok {
				for key, val := range a {
					meta.Annotations[key] = val
				}
			}
		}
	}
	// Common metadata which isn't a part of kubernetes metadata
	meta.Annotations[AnnotationPrefix+annotationCreatedBy] = cMeta.CreatedBy
	meta.Annotations[AnnotationPrefix+annotationUpdatedBy] = cMeta.UpdatedBy
	// Only set the UpdateTimestamp metadata if it's non-zero
	if !cMeta.UpdateTimestamp.IsZero() {
		meta.Annotations[AnnotationPrefix+annotationUpdateTimestamp] = cMeta.UpdateTimestamp.Format(time.RFC3339Nano)
	}

	// The non-common metadata needs to be converted into annotations
	for k, v := range obj.CustomMetadata().MapFields() {
		if cfg.CustomMetadataIsAnyType {
			meta.Annotations[AnnotationPrefix+k] = toString(v)
		} else {
			meta.Annotations[AnnotationPrefix+k] = v.(string) // nolint: revive
		}
	}

	return meta
}

func toString(t any) string {
	v := reflect.ValueOf(t)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String, reflect.Int, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64, reflect.Bool:
		return fmt.Sprintf("%v", v.Interface())
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return "" // Invalid kind to encode
	default:
		bytes, _ := json.Marshal(t)
		return string(bytes)
	}
}

func unmarshalKubernetesAdmissionReview(bytes []byte, format resource.WireFormat) (*admission.AdmissionReview, error) {
	if format != resource.WireFormatJSON {
		return nil, fmt.Errorf("unsupported WireFormat '%s'", fmt.Sprint(format))
	}

	rev := admission.AdmissionReview{}
	err := json.Unmarshal(bytes, &rev)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

func translateKubernetesAdmissionRequest(req *admission.AdmissionRequest, sch resource.Schema) (*resource.AdmissionRequest, error) {
	var obj, old resource.Object

	if len(req.Object.Raw) > 0 {
		obj = sch.ZeroValue()
		err := rawToObject(req.Object.Raw, obj)
		if err != nil {
			return nil, err
		}
	}
	if len(req.OldObject.Raw) > 0 {
		old = sch.ZeroValue()
		err := rawToObject(req.OldObject.Raw, old)
		if err != nil {
			return nil, err
		}
	}

	var action resource.AdmissionAction
	switch req.Operation {
	case admission.Create:
		action = resource.AdmissionActionCreate
	case admission.Update:
		action = resource.AdmissionActionUpdate
	case admission.Delete:
		action = resource.AdmissionActionDelete
	case admission.Connect:
		action = resource.AdmissionActionConnect
	}

	return &resource.AdmissionRequest{
		Action:  action,
		Kind:    req.Kind.Kind,
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		UserInfo: resource.AdmissionUserInfo{
			Username: req.UserInfo.Username,
			UID:      req.UserInfo.UID,
			Groups:   req.UserInfo.Groups,
		},
		Object:    obj,
		OldObject: old,
	}, nil
}
