package simple

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestSharedWatches_concurrentRegistration(t *testing.T) {
	req := operator.InformerRequest{
		Kind: testKind(),
		Supplier: func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			return &mockInformer{}, nil
		},
	}
	var added sync.WaitGroup
	add := func(operator.Informer, string) error {
		added.Done()
		return nil
	}

	var watches sharedWatches
	var registering sync.WaitGroup
	for i := 0; i < 32; i++ {
		added.Add(1)
		registering.Add(1)
		go func() {
			defer registering.Done()
			require.NoError(t, watches.register("foo/v1, Kind=Bar", req, add))
		}()
	}
	registering.Wait()
	added.Wait()

	assert.Len(t, watches.byKey["foo/v1, Kind=Bar"], 32)
}
