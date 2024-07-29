package k8s

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseKubernetesError(t *testing.T) {
	tests := []struct {
		name        string
		bytes       []byte
		statusCode  int
		err         error
		expectedErr error
	}{{
		name:       "status error",
		bytes:      nil,
		statusCode: 0,
		err: &errors2.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		},
		expectedErr: NewServerResponseError(&errors2.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		}, http.StatusInternalServerError),
	}, {
		name:       "status error, code conflicts with returned response code",
		bytes:      nil,
		statusCode: http.StatusConflict,
		err: &errors2.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		},
		expectedErr: NewServerResponseError(&errors2.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		}, http.StatusInternalServerError),
	}, {
		name:       "status error, code conflicts with returned response code (status.Code 0)",
		bytes:      nil,
		statusCode: http.StatusConflict,
		err: &errors2.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
			},
		},
		expectedErr: NewServerResponseError(&errors2.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
			},
		}, http.StatusConflict),
	}, {
		name:        "other error type, body JSON",
		bytes:       []byte(`{"code":404,"message":"no way"}`),
		err:         fmt.Errorf("no way"),
		expectedErr: NewServerResponseError(fmt.Errorf("no way"), http.StatusNotFound),
	}, {
		name:        "other error type, body JSON, no status code",
		bytes:       []byte(`{"message":"no way"}`),
		err:         fmt.Errorf("no way 2"),
		expectedErr: fmt.Errorf("no way"),
	}, {
		name:        "other error type, no body JSON",
		err:         fmt.Errorf("nope"),
		expectedErr: fmt.Errorf("nope"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := parseKubernetesError(test.bytes, test.statusCode, test.err)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}
