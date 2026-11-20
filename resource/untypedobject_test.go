package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUntypedObject_GetCommonMetadata_CreatedByUpdatedBy(t *testing.T) {
	tests := []struct {
		name              string
		annotations       map[string]string
		expectedCreatedBy string
		expectedUpdatedBy string
	}{
		{name: "no annotations"},
		{
			name: "grafana.app keys",
			annotations: map[string]string{
				"grafana.app/createdBy": "service-account:abc",
				"grafana.app/updatedBy": "user:def",
			},
			expectedCreatedBy: "service-account:abc",
			expectedUpdatedBy: "user:def",
		},
		{
			name: "legacy grafana.com keys",
			annotations: map[string]string{
				AnnotationCreatedBy: "legacy-creator",
				AnnotationUpdatedBy: "legacy-updater",
			},
			expectedCreatedBy: "legacy-creator",
			expectedUpdatedBy: "legacy-updater",
		},
		{
			name: "grafana.app keys take precedence",
			annotations: map[string]string{
				"grafana.app/createdBy": "new",
				AnnotationCreatedBy:     "old",
				"grafana.app/updatedBy": "new",
				AnnotationUpdatedBy:     "old",
			},
			expectedCreatedBy: "new",
			expectedUpdatedBy: "new",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			obj := &UntypedObject{}
			obj.Annotations = test.annotations
			md := obj.GetCommonMetadata()
			assert.Equal(t, test.expectedCreatedBy, md.CreatedBy)
			assert.Equal(t, test.expectedUpdatedBy, md.UpdatedBy)
		})
	}
}
