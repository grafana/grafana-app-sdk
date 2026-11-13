package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchOptions_SatisfiedBy(t *testing.T) {
	var (
		apiWatch        = WatchOptions{Detail: WatchDetailFullObject, Triggers: WatchTriggerChange}
		resyncingWatch  = WatchOptions{Detail: WatchDetailFullObject, Triggers: WatchTriggerChange | WatchTriggerSchedule}
		identityBus     = WatchOptions{Detail: WatchDetailIdentity, Triggers: WatchTriggerChange | WatchTriggerSchedule}
		reList          = WatchOptions{Detail: WatchDetailFullObject, Triggers: WatchTriggerSchedule}
		needsSweepToo   = WatchOptions{Triggers: WatchTriggerChange | WatchTriggerSchedule}
		needsSweepAlone = WatchOptions{Triggers: WatchTriggerSchedule}
	)

	tests := []struct {
		name     string
		needs    WatchOptions
		provided WatchOptions
		want     bool
	}{
		{
			name:     "zero needs are satisfied by the API watch",
			provided: apiWatch,
			want:     true,
		},
		{
			name:     "zero needs are not satisfied by identities",
			provided: identityBus,
			want:     false,
		},
		{
			name:     "zero needs are not satisfied by a re-list, which does not deliver each change",
			provided: reList,
			want:     false,
		},
		{
			name:     "an identity need is satisfied by whole objects",
			needs:    WatchOptions{Detail: WatchDetailIdentity},
			provided: apiWatch,
			want:     true,
		},
		{
			name:     "needing a sweep as well is not satisfied by a watch which does not resync",
			needs:    needsSweepToo,
			provided: apiWatch,
			want:     false,
		},
		{
			name:     "needing a sweep as well is satisfied by a resyncing watch",
			needs:    needsSweepToo,
			provided: resyncingWatch,
			want:     true,
		},
		{
			name:     "needing a sweep as well is not satisfied by a re-list alone",
			needs:    needsSweepToo,
			provided: reList,
			want:     false,
		},
		{
			name:     "needing only a sweep is satisfied by a re-list",
			needs:    needsSweepAlone,
			provided: reList,
			want:     true,
		},
		{
			name:     "needing only a sweep is satisfied by a resyncing watch",
			needs:    needsSweepAlone,
			provided: resyncingWatch,
			want:     true,
		},
		{
			name:     "needing only a sweep is not satisfied by a watch which does not resync",
			needs:    needsSweepAlone,
			provided: apiWatch,
			want:     false,
		},
		{
			name:     "needing only a sweep of identities is satisfied by an identity bus which re-lists",
			needs:    WatchOptions{Detail: WatchDetailIdentity, Triggers: WatchTriggerSchedule},
			provided: identityBus,
			want:     true,
		},
		{
			name:     "an unrecognized detail is needed in full",
			needs:    WatchOptions{Detail: WatchDetail("other")},
			provided: identityBus,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.needs.SatisfiedBy(tt.provided))
		})
	}
}

func TestWatchOptions_String(t *testing.T) {
	assert.Equal(t, "detail=full-object triggers=change", WatchOptions{}.String())
	assert.Equal(t, "detail=identity triggers=change+schedule", WatchOptions{
		Detail:   WatchDetailIdentity,
		Triggers: WatchTriggerChange | WatchTriggerSchedule,
	}.String())
	assert.Equal(t, "detail=full-object triggers=schedule", WatchOptions{
		Triggers: WatchTriggerSchedule,
	}.String())
}

func TestInformerWatchOptions(t *testing.T) {
	t.Run("an informer which says nothing states no options", func(t *testing.T) {
		options, stated := InformerWatchOptions(&mockInformer{})
		assert.False(t, stated)
		assert.Equal(t, WatchOptions{}, options)
	})

	t.Run("an informer which says so states its own options", func(t *testing.T) {
		want := WatchOptions{Detail: WatchDetailFullObject, Triggers: WatchTriggerSchedule}
		options, stated := InformerWatchOptions(&optionedInformer{options: want})
		assert.True(t, stated)
		assert.Equal(t, want, options)
	})
}

type optionedInformer struct {
	mockInformer
	options WatchOptions
}

func (o *optionedInformer) WatchOptions() WatchOptions {
	return o.options
}

var _ InformerWrapper = (*ConcurrentInformer)(nil)

// Wrapping must neither turn a mechanism's statement into silence nor make silence read as the default, since the
// two are checked differently.
func TestInformerWatchOptions_ThroughAWrapper(t *testing.T) {
	t.Run("reads through to what the wrapped informer states", func(t *testing.T) {
		want := WatchOptions{Detail: WatchDetailIdentity, Triggers: WatchTriggerSchedule}
		ci, err := NewConcurrentInformer(&optionedInformer{options: want}, ConcurrentInformerOptions{})
		require.NoError(t, err)

		options, stated := InformerWatchOptions(ci)
		assert.True(t, stated)
		assert.Equal(t, want, options)
	})

	t.Run("wrapping an informer which says nothing still states nothing", func(t *testing.T) {
		ci, err := NewConcurrentInformer(&mockInformer{}, ConcurrentInformerOptions{})
		require.NoError(t, err)

		_, stated := InformerWatchOptions(ci)
		assert.False(t, stated)
	})
}
