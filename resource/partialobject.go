package resource

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ Object = &PartialObject{}

// PartialObject implements resource.Object but only actually contains metadata information, and the raw payload that was used for unmarshaling.
// This is useful in accelerating the unmarshal process that is done serially with a NegotiatedSerializer in kubernetes watch requests,
// but does consume more memory as the entire original payload is embedded to avoid needing to copy or attempt to understand the non-metadata fields.
type PartialObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Raw               []byte `json:"-"`
}

func NewPartialObject(raw []byte, decoder func([]byte, any) error) (*PartialObject, error) {
	p := PartialObject{}
	if err := decoder(raw, &p); err != nil {
		return nil, err
	}
	p.Raw = raw
	return &p, nil
}

type metadataOnlyObject struct {
	*metav1.TypeMeta   `json:",inline"`
	*metav1.ObjectMeta `json:"metadata"`
}

func (p *PartialObject) UnmarshalJSON(b []byte) error {
	md := metadataOnlyObject{}
	if err := json.Unmarshal(b, &md); err != nil {
		return err
	}
	p.TypeMeta = *md.TypeMeta
	p.ObjectMeta = *md.ObjectMeta
	p.Raw = b
	return nil
}

func (p *PartialObject) GetRaw() []byte {
	return p.Raw
}

func (p *PartialObject) DeepCopyObject() runtime.Object {
	return p.Copy()
}

func (p *PartialObject) GetSpec() any {
	return nil
}

func (p *PartialObject) SetSpec(a any) error {
	return fmt.Errorf("spec cannot be set on a PartialObject")
}

func (p *PartialObject) GetSubresources() map[string]any {
	return map[string]any{}
}

func (p *PartialObject) GetSubresource(s string) (any, bool) {
	return nil, false
}

func (p *PartialObject) SetSubresource(key string, val any) error {
	return fmt.Errorf("subresource cannot be set on a PartialObject")
}

func (p *PartialObject) GetStaticMetadata() StaticMetadata {
	return StaticMetadata{
		Name:      p.ObjectMeta.Name,
		Namespace: p.ObjectMeta.Namespace,
		Group:     p.GroupVersionKind().Group,
		Version:   p.GroupVersionKind().Version,
		Kind:      p.GroupVersionKind().Kind,
	}
}

func (p *PartialObject) SetStaticMetadata(metadata StaticMetadata) {
	p.Name = metadata.Name
	p.Namespace = metadata.Namespace
	p.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   metadata.Group,
		Version: metadata.Version,
		Kind:    metadata.Kind,
	})
}

func (p *PartialObject) GetCommonMetadata() CommonMetadata {
	var err error
	dt := p.DeletionTimestamp
	var deletionTimestamp *time.Time
	if dt != nil {
		deletionTimestamp = &dt.Time
	}
	updt := time.Time{}
	createdBy := ""
	updatedBy := ""
	if p.Annotations != nil {
		strUpdt, ok := p.Annotations[AnnotationUpdateTimestamp]
		if ok {
			updt, err = time.Parse(time.RFC3339, strUpdt)
			if err != nil {
				// HMMMM
			}
		}
		createdBy = p.Annotations[AnnotationCreatedBy]
		updatedBy = p.Annotations[AnnotationUpdatedBy]
	}
	return CommonMetadata{
		UID:               string(p.UID),
		ResourceVersion:   p.ResourceVersion,
		Generation:        p.Generation,
		Labels:            p.Labels,
		CreationTimestamp: p.CreationTimestamp.Time,
		DeletionTimestamp: deletionTimestamp,
		Finalizers:        p.Finalizers,
		UpdateTimestamp:   updt,
		CreatedBy:         createdBy,
		UpdatedBy:         updatedBy,
		// TODO: populate ExtraFields in PartialObject?
	}
}

func (p *PartialObject) SetCommonMetadata(metadata CommonMetadata) {
	p.UID = types.UID(metadata.UID)
	p.ResourceVersion = metadata.ResourceVersion
	p.Generation = metadata.Generation
	p.Labels = metadata.Labels
	p.CreationTimestamp = metav1.NewTime(metadata.CreationTimestamp)
	if metadata.DeletionTimestamp != nil {
		dt := metav1.NewTime(*metadata.DeletionTimestamp)
		p.DeletionTimestamp = &dt
	} else {
		p.DeletionTimestamp = nil
	}
	p.Finalizers = metadata.Finalizers
	if p.Annotations == nil {
		p.Annotations = make(map[string]string)
	}
	if !metadata.UpdateTimestamp.IsZero() {
		p.Annotations[AnnotationUpdateTimestamp] = metadata.UpdateTimestamp.Format(time.RFC3339)
	}
	if metadata.CreatedBy != "" {
		p.Annotations[AnnotationCreatedBy] = metadata.CreatedBy
	}
	if metadata.UpdatedBy != "" {
		p.Annotations[AnnotationUpdatedBy] = metadata.UpdatedBy
	}
}

func (p *PartialObject) Copy() Object {
	cpy := PartialObject{}
	cpy.TypeMeta = metav1.TypeMeta{
		Kind:       p.Kind,
		APIVersion: p.APIVersion,
	}
	p.ObjectMeta.DeepCopyInto(&cpy.ObjectMeta)
	cpy.Raw = make([]byte, len(p.Raw))
	copy(cpy.Raw, p.Raw)
	return &cpy
}
