package openapiutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
)

func TestMergePaths(t *testing.T) {
	op := func(id string) *spec3.Operation {
		return &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: id}}
	}

	t.Run("empty source returns target unchanged", func(t *testing.T) {
		got, err := MergePaths(nil, nil)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("nil target is allocated", func(t *testing.T) {
		got, err := MergePaths(nil, map[string]spec3.PathProps{"/foo": {Get: op("getFoo")}})
		require.NoError(t, err)
		require.Contains(t, got, "/foo")
		assert.Equal(t, "getFoo", got["/foo"].Get.OperationId)
	})

	t.Run("source operations override target per method", func(t *testing.T) {
		target := map[string]spec3.PathProps{
			"/foo": {Get: op("oldGet"), Post: op("keptPost")},
			"/bar": {Get: op("untouched")},
		}
		got, err := MergePaths(target, map[string]spec3.PathProps{
			"/foo": {Get: op("newGet")},
		})
		require.NoError(t, err)
		assert.Equal(t, "newGet", got["/foo"].Get.OperationId)
		assert.Equal(t, "keptPost", got["/foo"].Post.OperationId)
		assert.Equal(t, "untouched", got["/bar"].Get.OperationId)
	})
}
