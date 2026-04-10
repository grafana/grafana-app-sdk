package simple

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	shardFilterDecisionProcessed = "processed"
	shardFilterDecisionSkipped   = "skipped"
	shardFilterDecisionError     = "error"
)

func newShardFilterDecisions(namespace string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "shard_filter",
		Name:      "decisions_total",
		Help:      "Total number of shard filter decisions by outcome.",
	}, []string{"decision", "event_type", "group", "version", "resource"})
}

type shardFilteredReconciler struct {
	group       string
	version     string
	resource    string
	metrics     *prometheus.CounterVec
	shardFilter operator.ShardFilter
	reconciler  operator.Reconciler
}

func newShardFilteredReconciler(kind resource.Kind, shardMetrics *prometheus.CounterVec, shardFilter operator.ShardFilter, reconciler operator.Reconciler) operator.Reconciler {
	return &shardFilteredReconciler{
		group:       kind.Group(),
		version:     kind.Version(),
		resource:    kind.Plural(),
		metrics:     shardMetrics,
		shardFilter: shardFilter,
		reconciler:  reconciler,
	}
}

func (s *shardFilteredReconciler) Reconcile(ctx context.Context, req operator.ReconcileRequest) (operator.ReconcileResult, error) {
	shouldProcess, err := s.shouldProcess(ctx, req.Object, reconcileActionLabel(req.Action))
	if err != nil {
		return operator.ReconcileResult{}, err
	}
	if !shouldProcess {
		return operator.ReconcileResult{}, nil
	}

	return s.reconciler.Reconcile(ctx, req)
}

func (s *shardFilteredReconciler) PrometheusCollectors() []prometheus.Collector {
	collectors := []prometheus.Collector{s.metrics}
	if provider, ok := s.reconciler.(metrics.Provider); ok {
		collectors = append(collectors, provider.PrometheusCollectors()...)
	}
	return collectors
}

type shardFilteredWatcher struct {
	group       string
	version     string
	resource    string
	metrics     *prometheus.CounterVec
	shardFilter operator.ShardFilter
	watcher     operator.ResourceWatcher
}

func newShardFilteredWatcher(kind resource.Kind, shardMetrics *prometheus.CounterVec, shardFilter operator.ShardFilter, watcher operator.ResourceWatcher) operator.ResourceWatcher {
	return &shardFilteredWatcher{
		group:       kind.Group(),
		version:     kind.Version(),
		resource:    kind.Plural(),
		metrics:     shardMetrics,
		shardFilter: shardFilter,
		watcher:     watcher,
	}
}

func (s *shardFilteredWatcher) Add(ctx context.Context, obj resource.Object) error {
	shouldProcess, err := s.shouldProcess(ctx, obj, string(operator.ResourceActionCreate))
	if err != nil || !shouldProcess {
		return err
	}

	return s.watcher.Add(ctx, obj)
}

func (s *shardFilteredWatcher) Update(ctx context.Context, src, tgt resource.Object) error {
	obj := tgt
	if obj == nil {
		// Some informer implementations only provide the previous object snapshot on update.
		// Use it for shard selection so the event can still be filtered consistently.
		obj = src
	}

	shouldProcess, err := s.shouldProcess(ctx, obj, string(operator.ResourceActionUpdate))
	if err != nil || !shouldProcess {
		return err
	}

	return s.watcher.Update(ctx, src, tgt)
}

func (s *shardFilteredWatcher) Delete(ctx context.Context, obj resource.Object) error {
	shouldProcess, err := s.shouldProcess(ctx, obj, string(operator.ResourceActionDelete))
	if err != nil || !shouldProcess {
		return err
	}

	return s.watcher.Delete(ctx, obj)
}

func (s *shardFilteredWatcher) PrometheusCollectors() []prometheus.Collector {
	collectors := []prometheus.Collector{s.metrics}
	if provider, ok := s.watcher.(metrics.Provider); ok {
		collectors = append(collectors, provider.PrometheusCollectors()...)
	}
	return collectors
}

func (s *shardFilteredWatcher) shouldProcess(ctx context.Context, obj resource.Object, eventType string) (bool, error) {
	return shouldProcessShardEvent(ctx, s.metrics, s.group, s.version, s.resource, eventType, s.shardFilter, obj)
}

func (s *shardFilteredReconciler) shouldProcess(ctx context.Context, obj resource.Object, eventType string) (bool, error) {
	return shouldProcessShardEvent(ctx, s.metrics, s.group, s.version, s.resource, eventType, s.shardFilter, obj)
}

func shouldProcessShardEvent(
	ctx context.Context,
	counter *prometheus.CounterVec,
	group string,
	version string,
	resourceName string,
	eventType string,
	shardFilter operator.ShardFilter,
	obj resource.Object,
) (bool, error) {
	if obj == nil {
		err := operator.ErrNilObject
		recordShardDecision(ctx, counter, group, version, resourceName, eventType, shardFilterDecisionError, err)
		return false, err
	}

	shouldProcess, err := shardFilter.ShouldProcess(ctx, obj)
	if err != nil {
		recordShardDecision(ctx, counter, group, version, resourceName, eventType, shardFilterDecisionError, err)
		return false, err
	}

	decision := shardFilterDecisionProcessed
	if !shouldProcess {
		decision = shardFilterDecisionSkipped
	}
	recordShardDecision(ctx, counter, group, version, resourceName, eventType, decision, nil)
	return shouldProcess, nil
}

func recordShardDecision(ctx context.Context, counter *prometheus.CounterVec, group string, version string, resourceName string, eventType string, decision string, err error) {
	counter.WithLabelValues(decision, eventType, group, version, resourceName).Inc()

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(
		attribute.String("shard_filter.decision", decision),
		attribute.String("shard_filter.event_type", eventType),
		attribute.String("shard_filter.group", group),
		attribute.String("shard_filter.version", version),
		attribute.String("shard_filter.resource", resourceName),
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
}

func reconcileActionLabel(action operator.ReconcileAction) string {
	resourceAction := operator.ResourceActionFromReconcileAction(action)
	if resourceAction == "" {
		return "UNKNOWN"
	}
	return string(resourceAction)
}

var _ operator.Reconciler = &shardFilteredReconciler{}
var _ metrics.Provider = &shardFilteredReconciler{}
var _ operator.ResourceWatcher = &shardFilteredWatcher{}
var _ metrics.Provider = &shardFilteredWatcher{}
