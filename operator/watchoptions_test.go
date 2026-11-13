package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchOptions_SatisfiedBy(t *testing.T) {
	tests := []struct {
		name     string
		needs    WatchOptions
		provided WatchOptions
		want     bool
	}{
		{
			name: "zero needs are satisfied by a mechanism which says nothing",
			want: true,
		},
		{
			name:     "zero needs are not satisfied by identities",
			provided: WatchOptions{Detail: WatchDetailIdentity, Trigger: WatchTriggerChange},
			want:     false,
		},
		{
			name:     "zero needs are not satisfied by a schedule",
			provided: WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerSchedule},
			want:     false,
		},
		{
			name:     "an identity need is satisfied by whole objects",
			needs:    WatchOptions{Detail: WatchDetailIdentity},
			provided: WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerChange},
			want:     true,
		},
		{
			name:     "a schedule need is satisfied by a delivery per change",
			needs:    WatchOptions{Trigger: WatchTriggerSchedule},
			provided: WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerChange},
			want:     true,
		},
		{
			name:     "a re-list serves a kind needing whole objects on a schedule",
			needs:    WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerSchedule},
			provided: WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerSchedule},
			want:     true,
		},
		{
			name:     "a re-list does not serve a kind needing whole objects per change",
			needs:    WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerChange},
			provided: WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerSchedule},
			want:     false,
		},
		{
			name:     "an identity bus does not serve a kind needing whole objects on a schedule",
			needs:    WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerSchedule},
			provided: WatchOptions{Detail: WatchDetailIdentity, Trigger: WatchTriggerChange},
			want:     false,
		},
		{
			name:     "unrecognized values are needed in full and not assumed to be short",
			needs:    WatchOptions{Detail: WatchDetail("other"), Trigger: WatchTrigger("other")},
			provided: WatchOptions{Detail: WatchDetail("other"), Trigger: WatchTrigger("other")},
			want:     true,
		},
		{
			name:     "an unrecognized need is not satisfied by identities on a schedule",
			needs:    WatchOptions{Detail: WatchDetail("other"), Trigger: WatchTrigger("other")},
			provided: WatchOptions{Detail: WatchDetailIdentity, Trigger: WatchTriggerSchedule},
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
	assert.Equal(t, "detail=full-object trigger=change", WatchOptions{}.String())
	assert.Equal(t, "detail=identity trigger=schedule",
		WatchOptions{Detail: WatchDetailIdentity, Trigger: WatchTriggerSchedule}.String())
}

func TestInformerWatchOptions(t *testing.T) {
	t.Run("an informer which says nothing carries whole objects per change", func(t *testing.T) {
		want := WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerChange}
		assert.Equal(t, want, InformerWatchOptions(&mockInformer{}))
	})

	t.Run("an informer which says so provides its own options", func(t *testing.T) {
		want := WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerSchedule}
		assert.Equal(t, want, InformerWatchOptions(&optionedInformer{options: want}))
	})
}

type optionedInformer struct {
	mockInformer
	options WatchOptions
}

func (o *optionedInformer) WatchOptions() WatchOptions {
	return o.options
}

func TestConcurrentInformer_WatchOptions(t *testing.T) {
	t.Run("forwards what the wrapped informer provides", func(t *testing.T) {
		provided := WatchOptions{Detail: WatchDetailIdentity, Trigger: WatchTriggerSchedule}
		ci, err := NewConcurrentInformer(&optionedInformer{options: provided}, ConcurrentInformerOptions{})
		require.NoError(t, err)

		assert.Equal(t, provided, InformerWatchOptions(ci))
	})

	t.Run("wrapping an informer which says nothing still carries the object per change", func(t *testing.T) {
		ci, err := NewConcurrentInformer(&mockInformer{}, ConcurrentInformerOptions{})
		require.NoError(t, err)

		assert.Equal(t, WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerChange},
			InformerWatchOptions(ci))
	})
}
