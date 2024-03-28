package k8s

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewOpinionatedMutatingAdmissionController(t *testing.T) {
	t.Run("nil wrap", func(t *testing.T) {
		m := NewOpinionatedMutatingAdmissionController(nil)
		assert.Nil(t, m.Underlying)
	})

	t.Run("wrap", func(t *testing.T) {
		wrapped := &testMutatingAdmissionController{}
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
		mutateFunc  func(context.Context, *resource.AdmissionRequest) (*resource.MutatingResponse, error)
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
					},
				},
			},
			expected: &resource.MutatingResponse{
				UpdatedObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy:       "me",
							resource.AnnotationUpdateTimestamp: cTimestamp.Format(time.RFC3339),
						},
						Labels: map[string]string{
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			expected: &resource.MutatingResponse{
				UpdatedObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationUpdatedBy:       "you",
							resource.AnnotationUpdateTimestamp: cTimestamp.Format(time.RFC3339),
						},
						Labels: map[string]string{
							"foo":        "bar",
							versionLabel: "v1-1",
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NewOpinionatedMutatingAdmissionController(&testMutatingAdmissionController{
				MutateFunc: test.mutateFunc,
			}).Mutate(context.Background(), &test.request)
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
		wrapped := &testValidatingAdmissionController{}
		v := NewOpinionatedValidatingAdmissionController(wrapped)
		assert.Equal(t, wrapped, v.Underlying)
	})
}

func TestOpinionatedValidatingAdmissionController_Validate(t *testing.T) {
	cTimestamp := time.Now().Truncate(time.Second).UTC()
	admErr := NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "some_err")

	tests := []struct {
		name         string
		validateFunc func(context.Context, *resource.AdmissionRequest) error
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "someone",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+AnnotationPrefix+annotationCreatedBy), http.StatusBadRequest, ErrReasonFieldNotAllowed),
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
							resource.AnnotationUpdatedBy: "someone else",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+AnnotationPrefix+annotationUpdatedBy), http.StatusBadRequest, ErrReasonFieldNotAllowed),
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationUpdateTimestamp: time.Now().Format(time.RFC3339),
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+AnnotationPrefix+annotationUpdateTimestamp), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name: "add action, underlying failure",
			validateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
				return admErr
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
					},
				},
			},
			expected: admErr,
		},
		{
			name: "add action, success",
			validateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
				return nil
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionCreate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "someone",
						},
					},
				},
				OldObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+AnnotationPrefix+annotationCreatedBy), http.StatusBadRequest, ErrReasonFieldNotAllowed),
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
							resource.AnnotationUpdatedBy: "someone else",
						},
					},
				},
				OldObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+AnnotationPrefix+annotationUpdatedBy), http.StatusBadRequest, ErrReasonFieldNotAllowed),
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
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy:       "someone",
							resource.AnnotationUpdateTimestamp: time.Now().Format(time.RFC3339),
						},
					},
				},
				OldObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy:       "someone",
							resource.AnnotationUpdateTimestamp: time.Time{}.Format(time.RFC3339),
						},
					},
				},
			},
			expected: NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+AnnotationPrefix+annotationUpdateTimestamp), http.StatusBadRequest, ErrReasonFieldNotAllowed),
		},
		{
			name: "update action, underlying failure",
			validateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
				return admErr
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
						},
					},
				},
				OldObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
						},
					},
				},
			},
			expected: admErr,
		},
		{
			name: "update action, success",
			validateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
				return nil
			},
			request: resource.AdmissionRequest{
				Action:  resource.AdmissionActionUpdate,
				Version: "v1-0",
				UserInfo: resource.AdmissionUserInfo{
					Username: "me",
				},
				Object: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
						},
					},
				},
				OldObject: &TestResourceObject{
					ObjectMeta: metav1.ObjectMeta{
						CreationTimestamp: metav1.NewTime(cTimestamp),
						Annotations: map[string]string{
							resource.AnnotationCreatedBy: "me",
						},
					},
				},
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := NewOpinionatedValidatingAdmissionController(&testValidatingAdmissionController{
				ValidateFunc: test.validateFunc,
			}).Validate(context.Background(), &test.request)
			fmt.Println(err)
			assert.Equal(t, test.expected, err)
		})
	}
}
