package jennies

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToOpenAPIName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "go module path",
			input:    "github.com/grafana/grafana-app-sdk/resource.TypedSpecObject",
			expected: "com.github.grafana.grafana-app-sdk.resource.TypedSpecObject",
		},
		{
			name:     "k8s path",
			input:    "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta",
			expected: "io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta",
		},
		{
			name:     "no domain",
			input:    "mypkg/v1.Foo",
			expected: "mypkg.v1.Foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ToOpenAPIName(tt.input))
		})
	}
}
