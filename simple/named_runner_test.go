package simple

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
)

type noopRunnable struct{}

func (noopRunnable) Run(context.Context) error { return nil }

func TestApp_AddNamedRunnable(t *testing.T) {
	a := &App{
		runner:       app.NewMultiRunner(),
		namedRunners: map[string]app.Runnable{RunnerNameInformers: noopRunnable{}},
	}

	r := noopRunnable{}
	require.NoError(t, a.AddNamedRunnable("custom", r))
	assert.Contains(t, a.NamedRunners(), "custom")
	assert.Contains(t, a.NamedRunners(), RunnerNameInformers)
	assert.Len(t, a.runner.Runners, 1, "named runnable must also be part of Runner()")

	require.Error(t, a.AddNamedRunnable("custom", r), "duplicate name")
	require.Error(t, a.AddNamedRunnable(RunnerNameInformers, r), "reserved by informers")
	require.Error(t, a.AddNamedRunnable(RunnerNameAll, r), "reserved")
	require.Error(t, a.AddNamedRunnable("", r), "empty")
	require.Error(t, a.AddNamedRunnable("nil", nil), "nil runnable")

	var _ app.NamedRunnerProvider = a
}
