package main

import (
	"context"
	"errors"
	"hash/fnv"
	"testing"

	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestHashRingShardFilterShouldProcess(t *testing.T) {
	obj := testObject("default", "demo")
	readRing := &fakeShardRingReader{
		state: services.Running,
		set: ring.ReplicationSet{
			Instances: []ring.InstanceDesc{{Addr: "10.0.0.1:7946"}},
		},
	}
	filter := &HashRingShardFilter{
		readRing: readRing,
		instance: fakeShardRingInstance{addr: "10.0.0.1:7946"},
	}

	shouldProcess, err := filter.ShouldProcess(context.Background(), obj)

	require.NoError(t, err)
	require.True(t, shouldProcess)
	require.Equal(t, expectedShardHash(obj.GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()), readRing.lastKey)
}

func TestHashRingShardFilterSkipsForeignShard(t *testing.T) {
	filter := &HashRingShardFilter{
		readRing: &fakeShardRingReader{
			state: services.Running,
			set: ring.ReplicationSet{
				Instances: []ring.InstanceDesc{{Addr: "10.0.0.2:7946"}},
			},
		},
		instance: fakeShardRingInstance{addr: "10.0.0.1:7946"},
	}

	shouldProcess, err := filter.ShouldProcess(context.Background(), testObject("default", "demo"))

	require.NoError(t, err)
	require.False(t, shouldProcess)
}

func TestHashRingShardFilterReturnsLookupErrors(t *testing.T) {
	expectedErr := errors.New("lookup failed")
	filter := &HashRingShardFilter{
		readRing: &fakeShardRingReader{
			state: services.Running,
			err:   expectedErr,
		},
		instance: fakeShardRingInstance{addr: "10.0.0.1:7946"},
	}

	shouldProcess, err := filter.ShouldProcess(context.Background(), testObject("default", "demo"))

	require.False(t, shouldProcess)
	require.ErrorIs(t, err, expectedErr)
}

func TestHashRingShardFilterReturnsErrorWhenRingIsNotRunning(t *testing.T) {
	filter := &HashRingShardFilter{
		readRing: &fakeShardRingReader{state: services.Starting},
		instance: fakeShardRingInstance{addr: "10.0.0.1:7946"},
	}

	shouldProcess, err := filter.ShouldProcess(context.Background(), testObject("default", "demo"))

	require.False(t, shouldProcess)
	require.ErrorContains(t, err, "shard ring is not running")
}

func TestHashRingShardFilterReturnsErrorForNilObject(t *testing.T) {
	filter := &HashRingShardFilter{
		readRing: &fakeShardRingReader{state: services.Running},
		instance: fakeShardRingInstance{addr: "10.0.0.1:7946"},
	}

	shouldProcess, err := filter.ShouldProcess(context.Background(), nil)

	require.False(t, shouldProcess)
	require.ErrorContains(t, err, "object is required")
}

func TestHashShardKeyUsesNamespaceAndName(t *testing.T) {
	obj := testObject("default", "demo")

	require.Equal(t, expectedShardHash(obj.GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()), hashShardKey(obj))
}

type fakeShardRingReader struct {
	state   services.State
	set     ring.ReplicationSet
	err     error
	lastKey uint32
}

func (f *fakeShardRingReader) GetWithOptions(key uint32, _ ring.Operation, _ ...ring.Option) (ring.ReplicationSet, error) {
	f.lastKey = key
	if f.err != nil {
		return ring.ReplicationSet{}, f.err
	}
	return f.set, nil
}

func (f *fakeShardRingReader) State() services.State {
	return f.state
}

type fakeShardRingInstance struct {
	addr string
}

func (f fakeShardRingInstance) GetInstanceAddr() string {
	return f.addr
}

func testObject(namespace, name string) resource.Object {
	obj := &resource.TypedSpecObject[map[string]any]{}
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.grafana.app",
		Version: "v1",
		Kind:    "BasicCustomResource",
	})
	return obj
}

func expectedShardHash(kind, namespace, name string) uint32 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(kind))
	_, _ = hasher.Write([]byte("/"))
	_, _ = hasher.Write([]byte(namespace))
	_, _ = hasher.Write([]byte("/"))
	_, _ = hasher.Write([]byte(name))
	return hasher.Sum32()
}
