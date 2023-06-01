package k8s

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

var nilRO resource.Object = nil

// this is janky
//
//nolint:funlen
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
		if len(k) > len(annotationPrefix) && k[:len(annotationPrefix)] == annotationPrefix {
			meta[k[len(annotationPrefix):]] = v
		}
	}
	if _, ok := meta["updateTimestamp"]; !ok {
		meta["updateTimestamp"] = kubeObject.ObjectMetadata.CreationTimestamp.Format(time.RFC3339Nano)
	}
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
	cmd.Labels = kubeObject.ObjectMetadata.Labels
	cmd.UID = string(kubeObject.ObjectMetadata.UID)
	if cmd.ExtraFields == nil {
		cmd.ExtraFields = make(map[string]any)
	}
	cmd.Finalizers = kubeObject.ObjectMetadata.Finalizers
	cmd.ExtraFields["generation"] = kubeObject.ObjectMetadata.Generation
	if len(kubeObject.ObjectMetadata.OwnerReferences) > 0 {
		cmd.ExtraFields["ownerReferences"] = kubeObject.ObjectMetadata.OwnerReferences
	}
	if len(kubeObject.ObjectMetadata.ManagedFields) > 0 {
		cmd.ExtraFields["managedFields"] = kubeObject.ObjectMetadata.ManagedFields
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
	into.SetListMetadata(resource.ListMetadata{
		ResourceVersion:    um.Metadata.ResourceVersion,
		Continue:           um.Metadata.Continue,
		RemainingItemCount: um.Metadata.RemainingItemCount,
	})
	items := make([]resource.Object, 0)
	for _, item := range um.Items {
		parsed, err := itemParser(item)
		if err != nil {
			return err
		}
		items = append(items, parsed)
	}
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

func getV1ObjectMeta(obj resource.Object, cfg ClientConfig) metav1.ObjectMeta {
	cMeta := obj.CommonMetadata()
	meta := metav1.ObjectMeta{
		Name:            obj.StaticMetadata().Name,
		Namespace:       obj.StaticMetadata().Namespace,
		UID:             types.UID(cMeta.UID),
		ResourceVersion: cMeta.ResourceVersion,
		Labels:          cMeta.Labels,
		Finalizers:      cMeta.Finalizers,
		Annotations:     make(map[string]string),
	}
	// Rest of the metadata in ExtraFields
	for k, v := range cMeta.ExtraFields {
		switch strings.ToLower(k) {
		case "generation": // TODO: should generation be non-implementation-specific metadata?
			if i, ok := v.(int64); ok {
				meta.Generation = i
			}
			if i, ok := v.(int); ok {
				meta.Generation = int64(i)
			}
		case "ownerReferences":
			if o, ok := v.([]metav1.OwnerReference); ok {
				meta.OwnerReferences = o
			}
		case "managedFields":
			if m, ok := v.([]metav1.ManagedFieldsEntry); ok {
				meta.ManagedFields = m
			}
		}
	}
	// Common metadata which isn't a part of kubernetes metadata
	meta.Annotations[annotationPrefix+"createdBy"] = cMeta.CreatedBy
	meta.Annotations[annotationPrefix+"updatedBy"] = cMeta.UpdatedBy
	meta.Annotations[annotationPrefix+"updateTimestamp"] = cMeta.UpdateTimestamp.Format(time.RFC3339Nano)

	// The non-common metadata needs to be converted into annotations
	for k, v := range obj.CustomMetadata().MapFields() {
		if cfg.CustomMetadataIsAnyType {
			meta.Annotations[annotationPrefix+k] = toString(v)
		} else {
			meta.Annotations[annotationPrefix+k] = v.(string)
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
