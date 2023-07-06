package k8s

import (
	"testing"
)

func TestNewOpinionatedMutatingAdmissionController(t *testing.T) {
	// TODO
}

/*func TestOpinionatedMutatingAdmissionController_Mutate(t *testing.T) {
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
					Metadata: TestResourceObjectMetadata{},
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
						Value:     time.Time{}.Format(time.RFC3339Nano),
					}, {
						Path:      "/metadata/labels", // Set the internal version label to the version of the endpoint
						Operation: resource.PatchOpAdd,
						Value: map[string]string{
							versionLabel: "v1-0",
						},
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
							Labels: map[string]string{
								"foo": "bar",
							},
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
						Operation: resource.PatchOpAdd,
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

func TestNewOpinionatedValidatingAdmissionController(t *testing.T) {
	// TODO
}

func TestOpinionatedValidatingAdmissionController_Validate(t *testing.T) {
	// TODO
}*/
