package k8s

import (
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
)

func TestNewMutatingResponseFromChange(t *testing.T) {
	tests := []struct {
		name     string
		from     resource.Object
		to       resource.Object
		expected *resource.MutatingResponse
		err      error
	}{
		{
			name: "simple change",
			from: &TestResourceObject{
				Spec: TestResourceSpec{
					StringField: "foo",
					FloatField:  1.2,
				},
			},
			to: &TestResourceObject{
				Spec: TestResourceSpec{
					StringField: "bar",
					FloatField:  1.2,
				},
			},
			expected: &resource.MutatingResponse{
				PatchOperations: []resource.PatchOperation{
					{
						Path:      "/spec/stringField",
						Operation: resource.PatchOpReplace,
						Value:     "bar",
					},
				},
			},
		},
		{
			name: "complex change",
			from: &TestResourceObject{
				Metadata: TestResourceObjectMetadata{
					CommonMetadata: resource.CommonMetadata{
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					CustomField1: "foo",
					CustomField2: "bar",
				},
				Spec: TestResourceSpec{
					StringField: "foo",
					FloatField:  1.2,
				},
			},
			to: &TestResourceObject{
				Spec: TestResourceSpec{
					StringField: "foo",
					FloatField:  1.3,
				},
				Metadata: TestResourceObjectMetadata{
					CommonMetadata: resource.CommonMetadata{
						Labels: map[string]string{
							"foo1": "bar1",
						},
						Finalizers: []string{"finalizer"},
					},
					CustomField1: "foo2",
					CustomField2: "bar2",
				},
			},
			expected: &resource.MutatingResponse{
				PatchOperations: []resource.PatchOperation{
					{
						Path:      "/metadata/labels/foo",
						Operation: resource.PatchOpRemove,
					},
					{
						Path:      "/metadata/labels/foo1",
						Operation: resource.PatchOpAdd,
						Value:     "bar1",
					},
					{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpAdd,
						Value:     []any{"finalizer"},
					},
					{
						Path:      "/metadata/annotations/grafana.com~1customField1",
						Operation: resource.PatchOpReplace,
						Value:     "foo2",
					},
					{
						Path:      "/metadata/annotations/grafana.com~1customField2",
						Operation: resource.PatchOpReplace,
						Value:     "bar2",
					},
					{
						Path:      "/spec/floatField",
						Operation: resource.PatchOpReplace,
						Value:     1.3,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NewMutatingResponseFromChange(test.from, test.to)
			assert.Equal(t, test.err, err)
			if test.expected != nil {
				assert.ElementsMatch(t, test.expected.PatchOperations, actual.PatchOperations)
			} else {
				assert.Nil(t, actual)
			}
		})
	}
}

func TestNewOpinionatedMutatingAdmissionController(t *testing.T) {

}

func TestOpinionatedMutatingAdmissionController_Mutate(t *testing.T) {
	// Hard-code the now() function
	nowTime := time.Now()
	now = func() time.Time {
		return nowTime
	}

	tests := []struct {
		name        string
		mutateFunc  func(*resource.AdmissionRequest) (*resource.MutatingResponse, error)
		request     resource.AdmissionRequest
		expected    *resource.MutatingResponse
		expectedErr error
	}{
		{
			name:       "no underlying, add action",
			mutateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: nowTime,
						},
					},
				},
			},
			expected: &resource.MutatingResponse{
				PatchOperations: []resource.PatchOperation{
					{
						Path:      "/metadata/createdBy", // Set createdBy to the request user
						Operation: resource.PatchOpReplace,
						Value:     "me",
					}, {
						Path:      "/metadata/updateTimestamp", // Set the updateTimestamp to the creationTimestamp
						Operation: resource.PatchOpReplace,
						Value:     nowTime.Format(time.RFC3339Nano),
					}, {
						Path:      "/metadata/labels/" + versionLabel, // Set the internal version label to the version of the endpoint
						Operation: resource.PatchOpReplace,
						Value:     "v1-0",
					},
				},
			},
		},

		{
			name:       "no underlying, update action",
			mutateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-1",
				UserInfo: resource.AdmissionUserInfo{
					Username: "you",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: nowTime,
						},
					},
				},
			},
			expected: &resource.MutatingResponse{
				PatchOperations: []resource.PatchOperation{
					{
						Path:      "/metadata/updatedBy", // Set createdBy to the request user
						Operation: resource.PatchOpReplace,
						Value:     "you",
					}, {
						Path:      "/metadata/updateTimestamp", // Set the updateTimestamp to the creationTimestamp
						Operation: resource.PatchOpReplace,
						Value:     nowTime.Format(time.RFC3339Nano),
					}, {
						Path:      "/metadata/labels/" + versionLabel, // Set the internal version label to the version of the endpoint
						Operation: resource.PatchOpReplace,
						Value:     "v1-1",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NewOpinionatedMutatingAdmissionController(test.mutateFunc).Mutate(&test.request)
			assert.Equal(t, test.expectedErr, err)
			if test.expected != nil {
				assert.ElementsMatch(t, test.expected.PatchOperations, actual.PatchOperations)
			} else {
				assert.Nil(t, actual)
			}
		})
	}
}

func TestNewMutatingAdmissionHandler(t *testing.T) {

}

func TestMutatingAdmissionHandler_AddController(t *testing.T) {

}

func TestMutatingAdmissionHandler_HTTPHandler(t *testing.T) {

}

func TestNewValidatingAdmissionHandler(t *testing.T) {

}

func TestValidatingAdmissionHandler_AddController(t *testing.T) {

}

func TestValidatingAdmissionHandler_HTTPHandler(t *testing.T) {

}
