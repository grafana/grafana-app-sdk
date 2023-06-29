package k8s

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ resource.Object = &TestResourceObject{}

var createdTime = time.Now().Truncate(time.Second)
var updatedTime = time.Now()

// complexObject is a fully-filled-out TestResourceObject
var complexObject = TestResourceObject{
	StaticMeta: resource.StaticMetadata{
		Kind:      "complex",
		Group:     "grafana.com",
		Version:   "v1",
		Namespace: "ns",
		Name:      "foo",
	},
	Metadata: TestResourceObjectMetadata{
		CommonMetadata: resource.CommonMetadata{
			UID:               "abc",
			ResourceVersion:   "12345",
			Labels:            map[string]string{"foo": "bar"},
			CreationTimestamp: createdTime,
			Finalizers:        []string{"finalizer"},
			UpdateTimestamp:   updatedTime,
			CreatedBy:         "me",
			UpdatedBy:         "you",
			ExtraFields: map[string]any{
				"generation": int64(1),
				"annotations": map[string]string{
					fmt.Sprintf("%screatedBy", annotationPrefix):       "me",
					fmt.Sprintf("%supdatedBy", annotationPrefix):       "you",
					fmt.Sprintf("%supdateTimestamp", annotationPrefix): updatedTime.Format(time.RFC3339Nano),
					fmt.Sprintf("%scustomField1", annotationPrefix):    "foo",
					fmt.Sprintf("%scustomField2", annotationPrefix):    "bar",
				},
			},
		},
		CustomField1: "foo",
		CustomField2: "bar",
	},
	Spec: TestResourceSpec{
		StringField: "a string",
		IntField:    64,
		FloatField:  6.4,
		ObjectSlice: []TestResourceSpecInner{
			{
				Foo: "first",
				Bar: map[string]string{
					"key": "value",
				},
			},
			{
				Foo: "second",
				Bar: map[string]string{
					"key2": "value2",
				},
			},
		},
	},
}

func TestRawToObject(t *testing.T) {
	badJSON := []byte("not json")
	nilJSONErr := json.Unmarshal(nil, &struct{}{})
	emptyJSONErr := json.Unmarshal([]byte{}, &struct{}{})
	badJSONErr := json.Unmarshal(badJSON, &struct{}{})

	// complexJSON is JSON generated from a kubernetes version of complexObject,
	// so we can source the raw JSON from kubernetes' libraries, and ensure that the process
	// returns a TestResourceObject identical to complexObject
	complexJSON, _ := json.Marshal(testKubernetesObject{
		TypeMeta: metav1.TypeMeta{
			Kind: complexObject.StaticMeta.Kind,
			APIVersion: metav1.GroupVersion{
				Group:   complexObject.StaticMeta.Group,
				Version: complexObject.StaticMeta.Version,
			}.String(),
		},
		Metadata: metav1.ObjectMeta{
			Name:              complexObject.StaticMeta.Name,
			Namespace:         complexObject.StaticMeta.Namespace,
			UID:               types.UID(complexObject.Metadata.UID),
			ResourceVersion:   complexObject.Metadata.ResourceVersion,
			Labels:            complexObject.Metadata.Labels,
			CreationTimestamp: metav1.Time{complexObject.Metadata.CreationTimestamp},
			Finalizers:        complexObject.Metadata.Finalizers,
			Generation:        complexObject.Metadata.ExtraFields["generation"].(int64),
			Annotations: map[string]string{
				fmt.Sprintf("%screatedBy", annotationPrefix):       complexObject.Metadata.CreatedBy,
				fmt.Sprintf("%supdatedBy", annotationPrefix):       complexObject.Metadata.UpdatedBy,
				fmt.Sprintf("%supdateTimestamp", annotationPrefix): complexObject.Metadata.UpdateTimestamp.Format(time.RFC3339Nano),
				fmt.Sprintf("%scustomField1", annotationPrefix):    complexObject.Metadata.CustomField1,
				fmt.Sprintf("%scustomField2", annotationPrefix):    complexObject.Metadata.CustomField2,
			},
		},
		Spec:   complexObject.Spec,
		Status: complexObject.Status,
	})

	tests := []struct {
		name        string
		raw         []byte
		into        resource.Object
		expectedObj resource.Object
		expectedErr error
	}{
		{
			name:        "nil bytes",
			raw:         nil,
			into:        &TestResourceObject{},
			expectedObj: &TestResourceObject{},
			expectedErr: nilJSONErr,
		},
		{
			name:        "empty bytes",
			raw:         nil,
			into:        &TestResourceObject{},
			expectedObj: &TestResourceObject{},
			expectedErr: emptyJSONErr,
		},
		{
			name:        "bad JSON",
			raw:         badJSON,
			into:        &TestResourceObject{},
			expectedObj: &TestResourceObject{},
			expectedErr: badJSONErr,
		},
		{
			name:        "nil into",
			raw:         []byte("{}"),
			into:        nil,
			expectedObj: nil,
			expectedErr: fmt.Errorf("into cannot be nil"),
		},
		{
			name:        "full object",
			raw:         complexJSON,
			into:        &TestResourceObject{},
			expectedObj: &complexObject,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := rawToObject(test.raw, test.into)
			assert.Equal(t, test.expectedErr, err)
			// convert to JSON and compare, because otherwise time comparisons are tricky
			expected, err := json.Marshal(test.expectedObj)
			require.Nil(t, err)
			actual, err := json.Marshal(test.into)
			require.Nil(t, err)
			assert.JSONEq(t, string(expected), string(actual))
		})
	}
}

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
			into: &listImpl{},
			parser: func(bytes []byte) (resource.Object, error) {
				return nil, fmt.Errorf("I AM ERROR")
			},
			expectedList:  &listImpl{},
			expectedError: fmt.Errorf("I AM ERROR"),
		},
		{
			name: "success",
			raw:  completeListJSON,
			into: &listImpl{},
			parser: func(bytes []byte) (resource.Object, error) {
				// We're not testing the unmarshal of the objects, so let's just put the raw bytes of the list item
				// into the spec of a simpleobject. We can check against a SimpleObject with a spec of the bytes
				// in our expectedList.Items
				return &resource.SimpleObject[[]byte]{
					Spec: bytes,
				}, nil
			},
			expectedList: &listImpl{
				lmd: resource.ListMetadata{
					ResourceVersion:    completeList.Metadata.ResourceVersion,
					Continue:           completeList.Metadata.Continue,
					RemainingItemCount: completeList.Metadata.RemainingItemCount,
				},
				items: []resource.Object{
					&resource.SimpleObject[[]byte]{
						Spec: []byte(`["a"]`),
					}, &resource.SimpleObject[[]byte]{
						Spec: []byte(`["b"]`),
					}, &resource.SimpleObject[[]byte]{
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
			assert.Equal(t, test.expectedList.ListMetadata(), test.into.ListMetadata())
			// Compare list items as JSON, as the lists are slices of pointers and will be unequal
			expectedJSON, _ := json.Marshal(test.expectedList.ListItems())
			actualJSON, _ := json.Marshal(test.into.ListItems())
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

func TestMarshalJSON(t *testing.T) {
	// To ensure that possible kubernetes meta changes don't break us, we convert a kubernetes object to bytes to use.
	// This makes sure that the marshalJSON converts object bytes into the equivalent kubernetes object bytes
	complexBytes, _ := json.Marshal(testKubernetesObject{
		TypeMeta: metav1.TypeMeta{
			Kind: complexObject.StaticMeta.Kind,
			APIVersion: metav1.GroupVersion{
				Group:   complexObject.StaticMeta.Group,
				Version: complexObject.StaticMeta.Version,
			}.String(),
		},
		Metadata: metav1.ObjectMeta{
			Name:            complexObject.StaticMeta.Name,
			Namespace:       complexObject.StaticMeta.Namespace,
			UID:             types.UID(complexObject.Metadata.UID),
			ResourceVersion: complexObject.Metadata.ResourceVersion,
			Labels:          complexObject.Metadata.Labels,
			//CreationTimestamp: metav1.Time{complexObject.Metadata.CreationTimestamp},
			Finalizers: complexObject.Metadata.Finalizers,
			Generation: complexObject.Metadata.ExtraFields["generation"].(int64),
			Annotations: map[string]string{
				fmt.Sprintf("%screatedBy", annotationPrefix):       complexObject.Metadata.CreatedBy,
				fmt.Sprintf("%supdatedBy", annotationPrefix):       complexObject.Metadata.UpdatedBy,
				fmt.Sprintf("%supdateTimestamp", annotationPrefix): complexObject.Metadata.UpdateTimestamp.Format(time.RFC3339Nano),
				fmt.Sprintf("%scustomField1", annotationPrefix):    complexObject.Metadata.CustomField1,
				fmt.Sprintf("%scustomField2", annotationPrefix):    complexObject.Metadata.CustomField2,
			},
		},
		Spec:   complexObject.Spec,
		Status: complexObject.Status,
	})

	tests := []struct {
		name          string
		obj           resource.Object
		extraLabels   map[string]string
		config        ClientConfig
		expectedJSON  []byte
		expectedError error
	}{
		{
			name:          "nil obj",
			obj:           nil,
			extraLabels:   nil,
			config:        ClientConfig{},
			expectedJSON:  nil,
			expectedError: fmt.Errorf("obj cannot be nil"),
		},
		{
			name:          "complex object",
			obj:           &complexObject,
			extraLabels:   nil,
			config:        ClientConfig{},
			expectedJSON:  complexBytes,
			expectedError: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bytes, err := marshalJSON(test.obj, test.extraLabels, test.config)
			assert.Equal(t, test.expectedError, err)
			if test.expectedJSON != nil {
				assert.JSONEq(t, string(test.expectedJSON), string(bytes))
			} else {
				assert.Nil(t, bytes)
			}
		})
	}
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

type TestResourceObject struct {
	StaticMeta    resource.StaticMetadata
	Metadata      TestResourceObjectMetadata
	Spec          TestResourceSpec
	Status        TestResourceStatus
	UnmarshalFunc func(self *TestResourceObject, bytes resource.ObjectBytes, config resource.UnmarshalConfig) error `json:"-"`
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

func (tro *TestResourceObject) StaticMetadata() resource.StaticMetadata {
	return tro.StaticMeta
}

func (tro *TestResourceObject) SetStaticMetadata(in resource.StaticMetadata) {
	tro.StaticMeta = in
}

func (tro *TestResourceObject) CommonMetadata() resource.CommonMetadata {
	return tro.Metadata.CommonMetadata
}

func (tro *TestResourceObject) SetCommonMetadata(in resource.CommonMetadata) {
	tro.Metadata.CommonMetadata = in
}

func (tro *TestResourceObject) CustomMetadata() resource.CustomMetadata {
	return resource.SimpleCustomMetadata{
		"customField1": tro.Metadata.CustomField1,
		"customField2": tro.Metadata.CustomField2,
	}
}

func (tro *TestResourceObject) SpecObject() any {
	return tro.Spec
}

func (tro *TestResourceObject) Subresources() map[string]any {
	return map[string]any{
		"status": TestResourceStatus{},
	}
}

func (tro *TestResourceObject) Copy() resource.Object {
	return resource.CopyObject(tro)
}

func (tro *TestResourceObject) Unmarshal(b resource.ObjectBytes, cfg resource.UnmarshalConfig) error {
	if tro.UnmarshalFunc != nil {
		return tro.UnmarshalFunc(tro, b, cfg)
	}
	err := json.Unmarshal(b.Spec, &tro.Spec)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b.Metadata, &tro.Metadata)
	if err != nil {
		return err
	}
	if statusBytes, ok := b.Subresources["status"]; ok {
		err = json.Unmarshal(statusBytes, &tro.Status)
		if err != nil {
			return err
		}
	}
	return nil
}
