package k8s

import (
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ resource.Object = &TestResourceObject{}

var createdTime = time.Now().Truncate(time.Second)
var updatedTime = time.Now()

func TestRawToListWithParser(t *testing.T) {
	ric := int64(2)
	completeList := k8sListWithItems{
		TypeMeta: metav1.TypeMeta{},
		Metadata: metav1.ListMeta{
			ResourceVersion:    "12345",
			Continue:           "abc",
			RemainingItemCount: &ric,
		},
		Items: []json.RawMessage{[]byte(`["a"]`), []byte(`["b"]`), []byte(`["c"]`)},
	}
	completeListJSON, _ := json.Marshal(completeList)

	tests := []struct {
		name          string
		raw           []byte
		into          resource.ListObject
		parser        func([]byte) (resource.Object, error)
		expectedList  resource.ListObject
		expectedError error
	}{
		// TODO: add nil param test cases if this method becomes exported (possibility in the future)
		{
			name: "parser error",
			raw:  completeListJSON,
			into: &resource.UntypedList{},
			parser: func(bytes []byte) (resource.Object, error) {
				return nil, fmt.Errorf("I AM ERROR")
			},
			expectedList:  &resource.UntypedList{},
			expectedError: fmt.Errorf("I AM ERROR"),
		},
		{
			name: "success",
			raw:  completeListJSON,
			into: &resource.UntypedList{},
			parser: func(bytes []byte) (resource.Object, error) {
				// We're not testing the unmarshal of the objects, so let's just put the raw bytes of the list item
				// into the spec of a simpleobject. We can check against a SimpleObject with a spec of the bytes
				// in our expectedList.Items
				return &resource.TypedSpecObject[[]byte]{
					Spec: bytes,
				}, nil
			},
			expectedList: &resource.UntypedList{
				ListMeta: metav1.ListMeta{
					ResourceVersion:    completeList.Metadata.ResourceVersion,
					Continue:           completeList.Metadata.Continue,
					RemainingItemCount: completeList.Metadata.RemainingItemCount,
				},
				Items: []resource.Object{
					&resource.TypedSpecObject[[]byte]{
						Spec: []byte(`["a"]`),
					}, &resource.TypedSpecObject[[]byte]{
						Spec: []byte(`["b"]`),
					}, &resource.TypedSpecObject[[]byte]{
						Spec: []byte(`["c"]`),
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := rawToListWithParser(test.raw, test.into, test.parser)
			assert.Equal(t, test.expectedError, err)
			assert.Equal(t, test.expectedList.GetSelfLink(), test.into.GetSelfLink())
			assert.Equal(t, test.expectedList.GetContinue(), test.into.GetContinue())
			assert.Equal(t, test.expectedList.GetRemainingItemCount(), test.into.GetRemainingItemCount())
			assert.Equal(t, test.expectedList.GetResourceVersion(), test.into.GetResourceVersion())
			assert.Equal(t, test.expectedList.GetObjectKind().GroupVersionKind(), test.into.GetObjectKind().GroupVersionKind())
			// Compare list items as JSON, as the lists are slices of pointers and will be unequal
			expectedJSON, _ := json.Marshal(test.expectedList.GetItems())
			actualJSON, _ := json.Marshal(test.into.GetItems())
			assert.JSONEq(t, string(expectedJSON), string(actualJSON))
		})
	}
}

type testKubernetesObject struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta  `json:"metadata"`
	Spec            TestResourceSpec   `json:"spec"`
	Status          TestResourceStatus `json:"status"`
}

func TestMarshalJSONPatch(t *testing.T) {
	tests := []struct {
		name          string
		patch         resource.PatchRequest
		expectedJSON  []byte
		expectedError error
	}{
		{
			name:          "empty request",
			patch:         resource.PatchRequest{},
			expectedJSON:  []byte("null"),
			expectedError: nil,
		},
		{
			name: "zero-length operations",
			patch: resource.PatchRequest{
				Operations: []resource.PatchOperation{},
			},
			expectedJSON:  []byte("[]"),
			expectedError: nil,
		},
		{
			name: "try to replace entire metadata object",
			patch: resource.PatchRequest{
				Operations: []resource.PatchOperation{
					{
						Path:      "/metadata",
						Operation: resource.PatchOpReplace,
					},
				},
			},
			expectedJSON:  nil,
			expectedError: fmt.Errorf("cannot patch entire metadata object"),
		},
		{
			name: "non-kubernetes metadata keys",
			patch: resource.PatchRequest{
				Operations: []resource.PatchOperation{
					{
						Path:      "/metadata/createdBy",
						Operation: resource.PatchOpReplace,
						Value:     "foo",
					}, {
						Path:      "/metadata/customKey",
						Operation: resource.PatchOpReplace,
						Value:     "bar",
					},
				},
			},
			expectedJSON: []byte(`[
						{"path":"/metadata/annotations/grafana.com~1createdBy","op":"add","value":"foo"},
						{"path":"/metadata/annotations/grafana.com~1customKey","op":"add","value":"bar"}]`),
			expectedError: nil,
		},
		{
			name: "mixed metadata",
			patch: resource.PatchRequest{
				Operations: []resource.PatchOperation{
					{
						Path:      "/metadata/createdBy",
						Operation: resource.PatchOpReplace,
						Value:     "foo",
					}, {
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpAdd,
						Value:     "bar",
					},
				},
			},
			expectedJSON: []byte(`[
						{"path":"/metadata/annotations/grafana.com~1createdBy","op":"add","value":"foo"},
						{"path":"/metadata/finalizers","op":"add","value":"bar"}]`),
			expectedError: nil,
		},
		{
			name: "using extraFields",
			patch: resource.PatchRequest{
				Operations: []resource.PatchOperation{
					{
						Path:      "/metadata/extraFields/generation",
						Operation: resource.PatchOpReplace,
						Value:     "12345",
					}, {
						Path:      "/metadata/extraFields/managedFields/manager",
						Operation: resource.PatchOpReplace,
						Value:     "new",
					},
				},
			},
			expectedJSON: []byte(`[
						{"path":"/metadata/generation","op":"replace","value":"12345"},
						{"path":"/metadata/managedFields/manager","op":"replace","value":"new"}]`),
			expectedError: nil,
		},
		{
			name: "try to replace entire metadata/extraFields object",
			patch: resource.PatchRequest{
				Operations: []resource.PatchOperation{
					{
						Path:      "/metadata/extraFields",
						Operation: resource.PatchOpReplace,
					},
				},
			},
			expectedJSON:  nil,
			expectedError: fmt.Errorf("cannot patch entire extraFields, please patch fields in extraFields instead"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := marshalJSONPatch(test.patch)
			assert.Equal(t, test.expectedError, err)
			if test.expectedJSON == nil {
				assert.Nil(t, actual)
			} else {
				assert.JSONEq(t, string(test.expectedJSON), string(actual))
			}
		})
	}
}

type TestCodec struct {
	ReadFunc  func(reader io.Reader, object resource.Object) error
	WriteFunc func(writer io.Writer, object resource.Object) error
}

func (tc *TestCodec) Read(reader io.Reader, object resource.Object) error {
	if tc.ReadFunc != nil {
		return tc.ReadFunc(reader, object)
	}
	return nil
}

func (tc *TestCodec) Write(writer io.Writer, object resource.Object) error {
	if tc.WriteFunc != nil {
		return tc.WriteFunc(writer, object)
	}
	return nil
}

type TestResourceObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              TestResourceSpec   `json:"spec"`
	Status            TestResourceStatus `json:"status"`
}

type TestResourceObjectMetadata struct {
	resource.CommonMetadata `json:",inline"`
	CustomField1            string `json:"customField1"`
	CustomField2            string `json:"customField2"`
}

type TestResourceSpec struct {
	StringField string                  `json:"stringField"`
	IntField    int64                   `json:"intField"`
	FloatField  float64                 `json:"floatField"`
	ObjectSlice []TestResourceSpecInner `json:"objectSlice"`
}

type TestResourceSpecInner struct {
	Foo string            `json:"foo"`
	Bar map[string]string `json:"bar"`
}

type TestResourceStatus struct {
}

func (tro *TestResourceObject) GetStaticMetadata() resource.StaticMetadata {
	return resource.StaticMetadata{
		Name:      tro.GetName(),
		Namespace: tro.GetNamespace(),
		Group:     tro.GroupVersionKind().Group,
		Version:   tro.GroupVersionKind().Version,
		Kind:      tro.GroupVersionKind().Kind,
	}
}

func (tro *TestResourceObject) SetStaticMetadata(metadata resource.StaticMetadata) {
	tro.Name = metadata.Name
	tro.Namespace = metadata.Namespace
	tro.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   metadata.Group,
		Version: metadata.Version,
		Kind:    metadata.Kind,
	})
}

func (tro *TestResourceObject) GetCommonMetadata() resource.CommonMetadata {
	var err error
	dt := tro.DeletionTimestamp
	var deletionTimestamp *time.Time
	if dt != nil {
		deletionTimestamp = &dt.Time
	}
	updt := time.Time{}
	createdBy := ""
	updatedBy := ""
	if tro.Annotations != nil {
		strUpdt, ok := tro.Annotations[resource.AnnotationUpdateTimestamp]
		if ok {
			updt, err = time.Parse(time.RFC3339, strUpdt)
			if err != nil {
				// HMMMM
			}
		}
		createdBy = tro.Annotations[resource.AnnotationCreatedBy]
		updatedBy = tro.Annotations[resource.AnnotationUpdatedBy]
	}
	return resource.CommonMetadata{
		UID:               string(tro.UID),
		ResourceVersion:   tro.ResourceVersion,
		Generation:        tro.Generation,
		Labels:            tro.Labels,
		CreationTimestamp: tro.CreationTimestamp.Time,
		DeletionTimestamp: deletionTimestamp,
		Finalizers:        tro.Finalizers,
		UpdateTimestamp:   updt,
		CreatedBy:         createdBy,
		UpdatedBy:         updatedBy,
		// TODO: populate ExtraFields in UntypedObject?
	}
}

func (tro *TestResourceObject) SetCommonMetadata(metadata resource.CommonMetadata) {
	tro.UID = types.UID(metadata.UID)
	tro.ResourceVersion = metadata.ResourceVersion
	tro.Generation = metadata.Generation
	tro.Labels = metadata.Labels
	tro.CreationTimestamp = metav1.NewTime(metadata.CreationTimestamp)
	if metadata.DeletionTimestamp != nil {
		dt := metav1.NewTime(*metadata.DeletionTimestamp)
		tro.DeletionTimestamp = &dt
	} else {
		tro.DeletionTimestamp = nil
	}
	tro.Finalizers = metadata.Finalizers
	if tro.Annotations == nil {
		tro.Annotations = make(map[string]string)
	}
	if !metadata.UpdateTimestamp.IsZero() {
		tro.Annotations[resource.AnnotationUpdateTimestamp] = metadata.UpdateTimestamp.Format(time.RFC3339)
	}
	if metadata.CreatedBy != "" {
		tro.Annotations[resource.AnnotationCreatedBy] = metadata.CreatedBy
	}
	if metadata.UpdatedBy != "" {
		tro.Annotations[resource.AnnotationUpdatedBy] = metadata.UpdatedBy
	}
}

func (tro *TestResourceObject) GetCustomField1() string {
	if tro.ObjectMeta.Annotations == nil {
		return ""
	}
	return tro.ObjectMeta.Annotations["customField1"]
}

func (tro *TestResourceObject) SetCustomField1(val string) {
	if tro.ObjectMeta.Annotations == nil {
		tro.ObjectMeta.Annotations = make(map[string]string)
	}
	tro.ObjectMeta.Annotations["customField1"] = val
}

func (tro *TestResourceObject) GetCustomField2() string {
	if tro.ObjectMeta.Annotations == nil {
		return ""
	}
	return tro.ObjectMeta.Annotations["customField2"]
}

func (tro *TestResourceObject) SetCustomField2(val string) {
	if tro.ObjectMeta.Annotations == nil {
		tro.ObjectMeta.Annotations = make(map[string]string)
	}
	tro.ObjectMeta.Annotations["customField2"] = val
}

func (tro *TestResourceObject) GetSpec() any {
	return tro.Spec
}

func (tro *TestResourceObject) SetSpec(spec any) error {
	if cast, ok := spec.(TestResourceSpec); ok {
		tro.Spec = cast
	}
	return fmt.Errorf("wrong type for spec")
}

func (tro *TestResourceObject) GetSubresources() map[string]any {
	return map[string]any{
		"status": tro.Status,
	}
}

func (tro *TestResourceObject) GetSubresource(name string) (any, bool) {
	if name == "status" {
		return tro.Status, true
	}
	return nil, false
}

func (tro *TestResourceObject) SetSubresource(name string, value any) error {
	if name != "status" {
		return fmt.Errorf("unknown subresource")
	}
	if cast, ok := value.(TestResourceStatus); ok {
		tro.Status = cast
	}
	return fmt.Errorf("wrong type for spec")
}

func (tro *TestResourceObject) Copy() resource.Object {
	return resource.CopyObject(tro)
}

func (tro *TestResourceObject) DeepCopyObject() runtime.Object {
	return tro.Copy()
}

type TestResourceObjectList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []TestResourceObject
}

func (o *TestResourceObjectList) DeepCopyObject() runtime.Object {
	return o.Copy()
}

func (o *TestResourceObjectList) Copy() resource.ListObject {
	cpy := &TestResourceObjectList{
		TypeMeta: o.TypeMeta,
		Items:    make([]TestResourceObject, len(o.Items)),
	}
	o.ListMeta.DeepCopyInto(&cpy.ListMeta)
	for i := 0; i < len(o.Items); i++ {
		if item, ok := o.Items[i].Copy().(*TestResourceObject); ok {
			cpy.Items[i] = *item
		}
	}
	return cpy
}

func (o *TestResourceObjectList) GetItems() []resource.Object {
	items := make([]resource.Object, len(o.Items))
	for i := 0; i < len(o.Items); i++ {
		items[i] = &o.Items[i]
	}
	return items
}

func (o *TestResourceObjectList) SetItems(items []resource.Object) {
	o.Items = make([]TestResourceObject, len(items))
	for i := 0; i < len(items); i++ {
		o.Items[i] = *items[i].(*TestResourceObject)
	}
}
