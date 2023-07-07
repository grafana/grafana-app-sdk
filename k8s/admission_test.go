package k8s

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
)

func TestNewOpinionatedMutatingAdmissionController(t *testing.T) {
	t.Run("nil wrap", func(t *testing.T) {
		m := NewOpinionatedMutatingAdmissionController(nil)
		assert.Nil(t, m.Underlying)
	})

	t.Run("wrap", func(t *testing.T) {
		wrapped := &resource.SimpleMutatingAdmissionController{}
		m := NewOpinionatedMutatingAdmissionController(wrapped)
		assert.Equal(t, wrapped, m.Underlying)
	})
}

func TestOpinionatedMutatingAdmissionController_Mutate(t *testing.T) {
	nowTime := time.Now().UTC()
	cTimestamp := time.Now().Truncate(time.Second).UTC()
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
							CreationTimestamp: cTimestamp,
						},
					},
				},
			},
			expected: &resource.MutatingResponse{
				UpdatedObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
							UpdateTimestamp:   cTimestamp,
							Labels: map[string]string{
								versionLabel: "v1-0",
							},
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
							CreationTimestamp: cTimestamp,
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			},
			expected: &resource.MutatingResponse{
				UpdatedObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							UpdateTimestamp:   now(),
							UpdatedBy:         "you",
							Labels: map[string]string{
								"foo":        "bar",
								versionLabel: "v1-1",
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NewOpinionatedMutatingAdmissionController(&resource.SimpleMutatingAdmissionController{
				MutateFunc: test.mutateFunc,
			}).Mutate(&test.request)
			assert.Equal(t, test.expectedErr, err)
			if test.expected != nil {
				assert.Equal(t, test.expected.UpdatedObject, actual.UpdatedObject)
			} else {
				assert.Nil(t, actual)
			}
		})
	}
}

func TestNewOpinionatedValidatingAdmissionController(t *testing.T) {
	t.Run("nil wrap", func(t *testing.T) {
		v := NewOpinionatedValidatingAdmissionController(nil)
		assert.Nil(t, v.Underlying)
	})

	t.Run("wrap", func(t *testing.T) {
		wrapped := &resource.SimpleValidatingAdmissionController{}
		v := NewOpinionatedValidatingAdmissionController(wrapped)
		assert.Equal(t, wrapped, v.Underlying)
	})
}

func TestOpinionatedValidatingAdmissionController_Validate(t *testing.T) {
	cTimestamp := time.Now().Truncate(time.Second).UTC()
	admErr := NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "some_err")

	tests := []struct {
		name         string
		validateFunc func(*resource.AdmissionRequest) error
		request      resource.AdmissionRequest
		expected     error
	}{
		{
			name:         "no underlying, add action, invalid createdBy",
			validateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "someone",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"createdBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name:         "no underlying, add action, invalid updatedBy",
			validateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
							UpdatedBy:         "someone else",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updatedBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name:         "no underlying, add action, specified updateTimestamp",
			validateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							UpdateTimestamp:   time.Now(),
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updateTimestamp"), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name: "add action, underlying failure",
			validateFunc: func(request *resource.AdmissionRequest) error {
				return admErr
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
						},
					},
				},
			},
			expected: admErr,
		},
		{
			name: "add action, success",
			validateFunc: func(request *resource.AdmissionRequest) error {
				return nil
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
						},
					},
				},
			},
			expected: nil,
		},
		{
			name:         "no underlying, update action, invalid createdBy",
			validateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "someone",
						},
					},
				},
				OldObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot change /metadata/annotations/"+annotationPrefix+"createdBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name:         "no underlying, update action, invalid updatedBy",
			validateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
							UpdatedBy:         "someone else",
						},
					},
				},
				OldObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updatedBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name:         "no underlying, add action, specified updateTimestamp",
			validateFunc: nil,
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
							UpdateTimestamp:   time.Now(),
						},
					},
				},
				OldObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							UpdateTimestamp:   time.Time{},
							CreatedBy:         "me",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updateTimestamp"), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name: "update action, underlying failure",
			validateFunc: func(request *resource.AdmissionRequest) error {
				return admErr
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
						},
					},
				},
				OldObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
						},
					},
				},
			},
			expected: admErr,
		},
		{
			name: "update action, success",
			validateFunc: func(request *resource.AdmissionRequest) error {
				return nil
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
						},
					},
				},
				OldObject: &TestResourceObject{
					Metadata: TestResourceObjectMetadata{
						CommonMetadata: resource.CommonMetadata{
							CreationTimestamp: cTimestamp,
							CreatedBy:         "me",
						},
					},
				},
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := NewOpinionatedValidatingAdmissionController(&resource.SimpleValidatingAdmissionController{
				ValidateFunc: test.validateFunc,
			}).Validate(&test.request)
			assert.Equal(t, test.expected, err)
		})
	}
}
