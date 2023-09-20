package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	versionLabel = "grafana-app-sdk-resource-version"

	annotationPrefix = "grafana.com/"
)

// groupVersionClient is the underlying client both Client and SchemalessClient use.
// GroupVersion is the unit with which kubernetes rest.Interface clients exist, so at minimum,
// we require one rest.Interface for each unique GroupVersion.
type groupVersionClient struct {
	client           rest.Interface
	version          string
	config           ClientConfig
	requestDurations *prometheus.HistogramVec
	totalRequests    *prometheus.CounterVec
}

func (g *groupVersionClient) get(ctx context.Context, identifier resource.Identifier, plural string,
	into resource.Object) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-get")
	defer span.End()
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	start := time.Now()
	bytes, err := request.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodGet, request.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodGet, request.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = rawToObject(bytes, into)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (g *groupVersionClient) getMetadata(ctx context.Context, identifier resource.Identifier, plural string) (
	*k8sObject, error) {
	ctx, span := GetTracer().Start(ctx, "kubernetes-getmetadata")
	defer span.End()
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	start := time.Now()
	bytes, err := request.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodGet, request.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodGet, request.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	md := k8sObject{}
	err = json.Unmarshal(bytes, &md)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to unmarshal request body: %s", err.Error()))
		return nil, err
	}
	return &md, nil
}

//nolint:unused
func (g *groupVersionClient) exists(ctx context.Context, identifier resource.Identifier, plural string) (
	bool, error) {
	ctx, span := GetTracer().Start(ctx, "kubernetes-exists")
	defer span.End()
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	start := time.Now()
	err := request.Do(ctx).StatusCode(&sc).Error()
	g.logRequestDuration(time.Since(start), sc, http.MethodGet, request.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodGet, request.URL().Path)
	if err != nil {
		// HTTP error?
		if sc == http.StatusNotFound {
			return false, nil
		}
		if sc > 0 {
			span.SetStatus(codes.Error, err.Error())
			return false, NewServerResponseError(err, sc)
		}
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	return true, nil
}

func (g *groupVersionClient) create(ctx context.Context, plural string, obj resource.Object,
	into resource.Object) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-create")
	defer span.End()
	bytes, err := marshalJSON(obj, map[string]string{
		versionLabel: g.version,
	}, g.config)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("error marshaling kubernetes JSON: %s", err.Error()))
		return err
	}

	sc := 0
	request := g.client.Post().Resource(plural).Body(bytes)
	if strings.TrimSpace(obj.StaticMetadata().Namespace) != "" {
		request = request.Namespace(obj.StaticMetadata().Namespace)
	}
	start := time.Now()
	bytes, err = request.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodPost, request.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPost),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodPost, request.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = rawToObject(bytes, into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

func (g *groupVersionClient) update(ctx context.Context, plural string, obj resource.Object,
	into resource.Object, _ resource.UpdateOptions) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-update")
	defer span.End()
	bytes, err := marshalJSON(obj, map[string]string{
		versionLabel: g.version,
	}, g.config)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("error marshaling kubernetes JSON: %s", err.Error()))
		return err
	}

	req := g.client.Put().Resource(plural).
		Name(obj.StaticMetadata().Name).Body(bytes)
	if strings.TrimSpace(obj.StaticMetadata().Namespace) != "" {
		req = req.Namespace(obj.StaticMetadata().Namespace)
	}
	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodPut, req.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPut),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodPut, req.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = rawToObject(raw, into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

func (g *groupVersionClient) updateSubresource(ctx context.Context, plural, subresource string, obj resource.Object,
	into resource.Object, _ resource.UpdateOptions) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-update-subresource")
	defer span.End()
	bytes, err := marshalJSON(obj, map[string]string{
		versionLabel: g.version,
	}, g.config)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("error marshaling kubernetes JSON: %s", err.Error()))
		return err
	}

	req := g.client.Put().Resource(plural).SubResource(subresource).
		Name(obj.StaticMetadata().Name).Body(bytes)
	if strings.TrimSpace(obj.StaticMetadata().Namespace) != "" {
		req = req.Namespace(obj.StaticMetadata().Namespace)
	}
	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodPut, req.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPut),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodPut, req.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = rawToObject(raw, into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

//nolint:revive,unused
func (g *groupVersionClient) patch(ctx context.Context, identifier resource.Identifier, plural string,
	patch resource.PatchRequest, into resource.Object, _ resource.PatchOptions) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-patch")
	defer span.End()
	bytes, err := marshalJSONPatch(patch)
	if err != nil {
		return err
	}
	req := g.client.Patch(types.JSONPatchType).Resource(plural).
		Name(identifier.Name).Body(bytes)
	if strings.TrimSpace(identifier.Namespace) != "" {
		req = req.Namespace(identifier.Namespace)
	}
	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodPatch, req.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPatch),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodPatch, req.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = rawToObject(raw, into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

func (g *groupVersionClient) delete(ctx context.Context, identifier resource.Identifier, plural string) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-delete")
	defer span.End()
	sc := 0
	request := g.client.Delete().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	start := time.Now()
	err := request.Do(ctx).StatusCode(&sc).Error()
	g.logRequestDuration(time.Since(start), sc, http.MethodDelete, request.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodDelete),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodDelete, request.URL().Path)
	if err != nil && sc >= 300 {
		return NewServerResponseError(err, sc)
	}
	return err
}

func (g *groupVersionClient) list(ctx context.Context, namespace, plural string, into resource.ListObject,
	options resource.ListOptions, itemParser func([]byte) (resource.Object, error)) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-list")
	defer span.End()
	req := g.client.Get().Resource(plural)
	if strings.TrimSpace(namespace) != "" {
		req = req.Namespace(namespace)
	}
	if len(options.LabelFilters) > 0 {
		req = req.Param("labelSelector", strings.Join(options.LabelFilters, ","))
	}
	if options.Limit > 0 {
		req = req.Param("limit", strconv.Itoa(options.Limit))
	}
	if options.Continue != "" {
		req = req.Param("continue", options.Continue)
	}
	sc := 0
	start := time.Now()
	bytes, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, http.MethodGet, req.URL().Path)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, http.MethodGet, req.URL().Path)
	if err != nil {
		err = parseKubernetesError(bytes, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return rawToListWithParser(bytes, into, itemParser)
}

//nolint:revive
func (g *groupVersionClient) watch(ctx context.Context, namespace, plural string,
	exampleObject resource.Object, options resource.WatchOptions) (*WatchResponse, error) {
	ctx, span := GetTracer().Start(ctx, "kubernetes-watch")
	defer span.End()
	g.client.Get()
	req := g.client.Get().Resource(plural).
		Param("watch", "1")
	if strings.TrimSpace(namespace) != "" {
		req = req.Namespace(namespace)
	}
	if len(options.LabelFilters) > 0 {
		req = req.Param("labelSelector", strings.Join(options.LabelFilters, ","))
	}
	if options.ResourceVersion != "" {
		req = req.Param("resourceVersion", options.ResourceVersion)
	}
	resp, err := req.Watch(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(
		attribute.Int("http.response.status_code", http.StatusOK),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(http.StatusOK, http.MethodGet, req.URL().Path)
	channelBufferSize := options.EventBufferSize
	if channelBufferSize <= 0 {
		channelBufferSize = 1
	}
	w := &WatchResponse{
		ex:     exampleObject,
		watch:  resp,
		ch:     make(chan resource.WatchEvent, channelBufferSize),
		stopCh: make(chan struct{}),
	}
	return w, nil
}

func (g *groupVersionClient) incRequestCounter(statusCode int, method string, path string) {
	if g.totalRequests == nil {
		return
	}

	g.totalRequests.WithLabelValues(strconv.Itoa(statusCode), method, path).Inc()
}

func (g *groupVersionClient) logRequestDuration(dur time.Duration, statusCode int, method string, path string) {
	if g.requestDurations == nil {
		return
	}

	g.requestDurations.WithLabelValues(strconv.Itoa(statusCode), method, path).Observe(dur.Seconds())
}

func (g *groupVersionClient) metrics() []prometheus.Collector {
	return []prometheus.Collector{
		g.totalRequests, g.requestDurations,
	}
}

// WatchResponse wraps a kubernetes watch.Interface in order to implement resource.WatchResponse.
// The underlying watch.Interface can be accessed with KubernetesWatch().
type WatchResponse struct {
	watch   watch.Interface
	ch      chan resource.WatchEvent
	stopCh  chan struct{}
	ex      resource.Object
	started bool
}

//nolint:revive,staticcheck
func (w *WatchResponse) start() {
	for {
		select {
		case evt := <-w.watch.ResultChan():
			var obj resource.Object
			if cast, ok := evt.Object.(intoObject); ok {
				obj = w.ex.Copy()
				err := cast.Into(obj)
				if err != nil {
					// TODO: hmm
					break
				}
			} else if cast, ok := evt.Object.(wrappedObject); ok {
				obj = cast.ResourceObject()
			} else {
				// TODO: hmm
			}
			w.ch <- resource.WatchEvent{
				EventType: string(evt.Type),
				Object:    obj,
			}
		case <-w.stopCh:
			close(w.stopCh)
			return
		}
	}
}

// Stop stops the translation channel between the kubernetes watch.Interface,
// and stops the continued watch request encapsulated by the watch.Interface.
func (w *WatchResponse) Stop() {
	w.stopCh <- struct{}{}
	close(w.ch)
	w.watch.Stop()
}

// WatchEvents returns a channel that receives watch events.
// All calls to this method will return the same channel.
// This channel will stop receiving events if KubernetesWatch() is called, as that halts the event translation process.
// If Stop() is called, ths channel is closed.
func (w *WatchResponse) WatchEvents() <-chan resource.WatchEvent {
	if !w.started {
		// Start the translation buffer
		go w.start()
	}
	return w.ch
}

// KubernetesWatch returns the underlying watch.Interface.
// Calling this method will shut down the translation channel between the watch.Interface and ResultChan().
// Using both KubernetesWatch() and ResultChan() simultaneously is not supported, and may result in undefined behavior.
func (w *WatchResponse) KubernetesWatch() watch.Interface {
	// Stop the internal channel with the translation layer
	if w.started {
		w.stopCh <- struct{}{}
	}
	return w.watch
}

type k8sErrBody struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// parseKubernetesError attempts to create a better error response from a k8s error
// by using the body and status code from the request. The normal k8s error string is very generic
// (typically: "the server rejected our request for an unknown reason (<METHOD> <GV> <NAME>)"),
// but the response body often has more details about the nature of the failure (for example, missing a required field).
// Ths method will parse the response body for a better error message if available, and return a *ServerResponseError
// if the status code is a non-success (>= 300).
func parseKubernetesError(responseBytes []byte, statusCode int, err error) error {
	if len(responseBytes) > 0 {
		parsed := k8sErrBody{}
		// If we can parse the response body, use the error contained there instead, because it's clearer
		if e := json.Unmarshal(responseBytes, &parsed); e == nil {
			err = fmt.Errorf(parsed.Message)
		}
	}
	// HTTP error?
	if statusCode >= 300 {
		return NewServerResponseError(err, statusCode)
	}
	return err
}
