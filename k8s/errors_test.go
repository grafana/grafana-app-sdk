package k8s

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		err: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		},
		expectedErr: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		},
	}, {
		name:       "status error, code conflicts with returned response code",
		bytes:      nil,
		statusCode: http.StatusConflict,
		err: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		},
		expectedErr: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
				Code:    http.StatusInternalServerError,
			},
		},
	}, {
		name:       "status error, code conflicts with returned response code (status.Code 0)",
		bytes:      nil,
		statusCode: http.StatusConflict,
		err: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
			},
		},
		expectedErr: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				Message: "I AM ERROR",
			},
		},
	}, {
		name: "kubernetes status response",
		bytes: []byte(`{
			"apiVersion":"v1",
			"kind":"Status",
			"code":404,
			"message":"no way"
		}`),
		err: fmt.Errorf("no way"),
		expectedErr: &apierrors.StatusError{
			ErrStatus: metav1.Status{
				TypeMeta: metav1.TypeMeta{
					APIVersion: StatusAPIVersion,
					Kind:       StatusKind,
				},
				Message: "no way",
				Code:    http.StatusNotFound,
			},
		},
	}, {
		name:        "other error type, body JSON, no status code",
		bytes:       []byte(`{"message":"no way"}`),
		err:         fmt.Errorf("no way 2"),
		expectedErr: NewServerResponseError(fmt.Errorf("no way 2"), http.StatusServiceUnavailable),
	}, {
		name:        "other error type, no body JSON",
		err:         fmt.Errorf("nope"),
		expectedErr: NewServerResponseError(fmt.Errorf("nope"), http.StatusServiceUnavailable),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ParseKubernetesError(test.bytes, test.statusCode, test.err)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestStatusFromError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   apierrors.APIStatus
		wantOk bool
	}{
		{
			name: "status error",
			err: &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Message: "I AM ERROR",
				},
			},
			want: &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Message: "I AM ERROR",
				},
			},
			wantOk: true,
		},
		{
			name:   "server response error",
			err:    NewServerResponseError(fmt.Errorf("I AM ERROR"), http.StatusNotFound),
			want:   NewServerResponseError(fmt.Errorf("I AM ERROR"), http.StatusNotFound),
			wantOk: true,
		},
		{
			name:   "other error type",
			err:    fmt.Errorf("I AM ERROR"),
			want:   NewServerResponseError(fmt.Errorf("I AM ERROR"), http.StatusInternalServerError),
			wantOk: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, ok := StatusFromError(test.err)
			assert.Equal(t, test.want, status)
			assert.Equal(t, test.wantOk, ok)
		})
	}
}
