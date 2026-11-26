package simple

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestSharedWatches_concurrentRegistration(t *testing.T) {
	const registrations = 32
	req := operator.InformerRequest{
		Kind: testKind(),
		Supplier: func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			return &mockInformer{}, nil
		},
	}
	var added atomic.Int32
	add := func(operator.Informer, string) error {
		added.Add(1)
		return nil
	}

	var watches sharedWatches
	var registering sync.WaitGroup
	errs := make(chan error, registrations)
	for i := 0; i < registrations; i++ {
		registering.Add(1)
		go func() {
			defer registering.Done()
			errs <- watches.register("foo/v1, Kind=Bar", req, add)
		}()
	}
	registering.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.EqualValues(t, registrations, added.Load())
	assert.Len(t, watches.byKey["foo/v1, Kind=Bar"], registrations)
}

type statedInformer struct {
	mockInformer
	provides operator.WatchOptions
}

func (s *statedInformer) WatchOptions() operator.WatchOptions { return s.provides }

func TestSharedWatches_recordsASnapshotOfTheDeclaration(t *testing.T) {
	const key = "foo/v1, Kind=Bar"
	request := func(live *operator.WatchLiveOptions, provided operator.WatchDetail) operator.InformerRequest {
		req := operator.InformerRequest{
			Kind: testKind(),
			Supplier: func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
				return &statedInformer{provides: operator.WatchOptions{
					Live: &operator.WatchLiveOptions{Detail: provided},
				}}, nil
			},
		}
		req.Options.WatchNeeds = operator.WatchOptions{Live: live}
		return req
	}
	add := func(operator.Informer, string) error { return nil }

	var watches sharedWatches
	reused := &operator.WatchLiveOptions{Detail: operator.WatchDetailFullObject}
	require.NoError(t, watches.register(key, request(reused, operator.WatchDetailFullObject), add))

	reused.Detail = operator.WatchDetailMetadata
	err := watches.register(key, request(reused, operator.WatchDetailMetadata), add)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Live deliveries carrying less than "full-object"`)
}
