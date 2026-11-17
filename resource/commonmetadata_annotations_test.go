package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The server-set grafana.app/ keys must win over the legacy grafana.com/ keys for every object wrapper,
// not just UntypedObject.
func TestCommonMetadata_CreatedByUpdatedBy_AllObjectTypes(t *testing.T) {
	annotationSets := map[string]struct {
		annotations map[string]string
		createdBy   string
		updatedBy   string
	}{
		"grafana.app keys": {
			annotations: map[string]string{"grafana.app/createdBy": "user:a", "grafana.app/updatedBy": "user:b"},
			createdBy:   "user:a",
			updatedBy:   "user:b",
		},
		"legacy keys": {
			annotations: map[string]string{AnnotationCreatedBy: "old-a", AnnotationUpdatedBy: "old-b"},
			createdBy:   "old-a",
			updatedBy:   "old-b",
		},
		"grafana.app wins": {
			annotations: map[string]string{
				"grafana.app/createdBy": "new", AnnotationCreatedBy: "old",
				"grafana.app/updatedBy": "new", AnnotationUpdatedBy: "old",
			},
			createdBy: "new",
			updatedBy: "new",
		},
	}

	for name, tc := range annotationSets {
		t.Run(name, func(t *testing.T) {
			typedSpec := &TypedSpecObject[string]{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations}}
			md := typedSpec.GetCommonMetadata()
			assert.Equal(t, tc.createdBy, md.CreatedBy, "TypedSpecObject createdBy")
			assert.Equal(t, tc.updatedBy, md.UpdatedBy, "TypedSpecObject updatedBy")

			typedStatus := &TypedSpecStatusObject[string, string]{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations}}
			md = typedStatus.GetCommonMetadata()
			assert.Equal(t, tc.createdBy, md.CreatedBy, "TypedSpecStatusObject createdBy")
			assert.Equal(t, tc.updatedBy, md.UpdatedBy, "TypedSpecStatusObject updatedBy")

			typed := &TypedObject[string, struct{}]{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations}}
			md = typed.GetCommonMetadata()
			assert.Equal(t, tc.createdBy, md.CreatedBy, "TypedObject createdBy")
			assert.Equal(t, tc.updatedBy, md.UpdatedBy, "TypedObject updatedBy")
		})
	}
}
