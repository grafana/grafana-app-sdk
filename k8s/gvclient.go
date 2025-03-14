package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	versionLabel = "grafana-app-sdk-resource-version"

	// AnnotationPrefix is the prefix used in annotations which contain grafana kind metadata
	AnnotationPrefix = "grafana.com/"
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
	into resource.Object, codec resource.Codec) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-get")
	defer span.End()
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	start := time.Now()
	raw, err := request.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "GET", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, "GET", plural, "spec")
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = codec.Read(bytes.NewReader(raw), into)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

type metadataObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

// dryRun=All

func (g *groupVersionClient) getMetadata(ctx context.Context, identifier resource.Identifier, plural string) (
	*metadataObject, error) {
	ctx, span := GetTracer().Start(ctx, "kubernetes-getmetadata")
	defer span.End()
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	start := time.Now()
	raw, err := request.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "GET", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, "GET", plural, "spec")
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	md := metadataObject{}
	err = json.Unmarshal(raw, &md)
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
	g.logRequestDuration(time.Since(start), sc, "GET", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, "GET", plural, "spec")
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

func (g *groupVersionClient) create(
	ctx context.Context,
	plural string,
	obj resource.Object,
	into resource.Object,
	opts resource.CreateOptions,
	codec resource.Codec,
) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-create")
	defer span.End()
	addLabels(obj, map[string]string{
		versionLabel: g.version,
	})
	buf := &bytes.Buffer{}
	err := codec.Write(buf, obj)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("error marshaling kubernetes JSON: %s", err.Error()))
		return err
	}

	sc := 0
	request := g.client.Post().Resource(plural).Body(buf.Bytes())
	if strings.TrimSpace(obj.GetNamespace()) != "" {
		request = request.Namespace(obj.GetNamespace())
	}

	if opts.DryRun {
		request = request.Param("dryRun", "All")
	}

	start := time.Now()
	raw, err := request.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "CREATE", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPost),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, "CREATE", plural, "spec")
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = codec.Read(bytes.NewReader(raw), into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

func (g *groupVersionClient) update(
	ctx context.Context,
	plural string,
	obj resource.Object,
	into resource.Object,
	opts resource.UpdateOptions,
	codec resource.Codec,
) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-update")
	defer span.End()
	addLabels(obj, map[string]string{
		versionLabel: g.version,
	})
	buf := &bytes.Buffer{}
	err := codec.Write(buf, obj)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("error marshaling kubernetes JSON: %s", err.Error()))
		return err
	}

	req := g.client.Put().Resource(plural).
		Name(obj.GetName()).Body(buf.Bytes())
	if strings.TrimSpace(obj.GetNamespace()) != "" {
		req = req.Namespace(obj.GetNamespace())
	}
	if opts.DryRun {
		req = req.Param("dryRun", "All")
	}
	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "UPDATE", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPut),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, "UPDATE", plural, "spec")
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = codec.Read(bytes.NewReader(raw), into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

func (g *groupVersionClient) updateSubresource(
	ctx context.Context,
	plural, subresource string,
	obj resource.Object,
	into resource.Object,
	opts resource.UpdateOptions,
	codec resource.Codec,
) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-update-subresource")
	defer span.End()
	addLabels(obj, map[string]string{
		versionLabel: g.version,
	})
	buf := &bytes.Buffer{}
	err := codec.Write(buf, obj)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("error marshaling kubernetes JSON: %s", err.Error()))
		return err
	}

	req := g.client.Put().Resource(plural).SubResource(subresource).
		Name(obj.GetName()).Body(buf.Bytes())
	if strings.TrimSpace(obj.GetNamespace()) != "" {
		req = req.Namespace(obj.GetNamespace())
	}

	if opts.DryRun {
		req = req.Param("dryRun", "All")
	}

	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "UPDATE", plural, subresource)
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPut),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, "UPDATE", plural, subresource)
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = codec.Read(bytes.NewReader(raw), into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

//nolint:revive,unused
func (g *groupVersionClient) patch(
	ctx context.Context,
	identifier resource.Identifier,
	plural string,
	patch resource.PatchRequest,
	into resource.Object,
	opts resource.PatchOptions,
	codec resource.Codec,
) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-patch")
	defer span.End()
	patchBytes, err := marshalJSONPatch(patch)
	if err != nil {
		return err
	}
	req := g.client.Patch(types.JSONPatchType).Resource(plural).
		Name(identifier.Name).Body(patchBytes)
	if strings.TrimSpace(identifier.Namespace) != "" {
		req = req.Namespace(identifier.Namespace)
	}
	if opts.DryRun {
		req = req.Param("dryRun", "All")
	}
	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "PATCH", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodPatch),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, "PATCH", plural, "spec")
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	err = codec.Read(bytes.NewReader(raw), into)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("unable to convert kubernetes response to resource: %s", err.Error()))
		return err
	}
	return nil
}

func (g *groupVersionClient) delete(ctx context.Context, identifier resource.Identifier, plural string, options resource.DeleteOptions) error {
	ctx, span := GetTracer().Start(ctx, "kubernetes-delete")
	defer span.End()
	sc := 0
	request := g.client.Delete().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	if options.Preconditions.ResourceVersion != "" {
		request = request.Param("preconditions.resourceVersion", options.Preconditions.ResourceVersion)
	}
	if options.Preconditions.UID != "" {
		request = request.Param("preconditions.uid", options.Preconditions.UID)
	}
	if options.PropagationPolicy != "" {
		request = request.Param("propagationPolicy", string(options.PropagationPolicy))
	}
	start := time.Now()
	err := request.Do(ctx).StatusCode(&sc).Error()
	g.logRequestDuration(time.Since(start), sc, "DELETE", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodDelete),
		attribute.String("server.address", request.URL().Hostname()),
		attribute.String("server.port", request.URL().Port()),
		attribute.String("url.full", request.URL().String()),
	)
	g.incRequestCounter(sc, "DELETE", plural, "spec")
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
	if len(options.FieldSelectors) > 0 {
		req = req.Param("fieldSelector", strings.Join(options.FieldSelectors, ","))
	}
	if options.Limit > 0 {
		req = req.Param("limit", strconv.Itoa(options.Limit))
	}
	if options.Continue != "" {
		req = req.Param("continue", options.Continue)
	}
	if options.ResourceVersion != "" {
		req = req.Param("resourceVersion", options.ResourceVersion)
	}
	sc := 0
	start := time.Now()
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	g.logRequestDuration(time.Since(start), sc, "LIST", plural, "spec")
	span.SetAttributes(
		attribute.Int("http.response.status_code", sc),
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("server.address", req.URL().Hostname()),
		attribute.String("server.port", req.URL().Port()),
		attribute.String("url.full", req.URL().String()),
	)
	g.incRequestCounter(sc, "LIST", plural, "spec")
	if err != nil {
		err = parseKubernetesError(raw, sc, err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return rawToListWithParser(raw, into, itemParser)
}

//nolint:revive
func (g *groupVersionClient) watch(ctx context.Context, namespace, plural string,
	exampleObject resource.Object, options resource.WatchOptions, codec resource.Codec) (*WatchResponse, error) {
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
	if len(options.FieldSelectors) > 0 {
		req = req.Param("fieldSelector", strings.Join(options.FieldSelectors, ","))
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
	g.incRequestCounter(http.StatusOK, "WATCH", plural, "spec")
	channelBufferSize := options.EventBufferSize
	if channelBufferSize <= 0 {
		channelBufferSize = 1
	}
	w := &WatchResponse{
		ex:     exampleObject,
		codec:  codec,
		watch:  resp,
		ch:     make(chan resource.WatchEvent, channelBufferSize),
		stopCh: make(chan struct{}),
	}
	return w, nil
}

func (g *groupVersionClient) incRequestCounter(statusCode int, verb, kind, subresource string) {
	if g.totalRequests == nil {
		return
	}

	g.totalRequests.WithLabelValues(strconv.Itoa(statusCode), verb, kind, subresource).Inc()
}

func (g *groupVersionClient) logRequestDuration(dur time.Duration, statusCode int, verb, kind, subresource string) {
	if g.requestDurations == nil {
		return
	}

	g.requestDurations.WithLabelValues(strconv.Itoa(statusCode), verb, kind, subresource).Observe(dur.Seconds())
}

func (g *groupVersionClient) metrics() []prometheus.Collector {
	return []prometheus.Collector{
		g.totalRequests, g.requestDurations,
	}
}

// WatchResponse wraps a kubernetes watch.Interface in order to implement resource.WatchResponse.
// The underlying watch.Interface can be accessed with KubernetesWatch().
type WatchResponse struct {
	watch    watch.Interface
	ch       chan resource.WatchEvent
	stopCh   chan struct{}
	ex       resource.Object
	codec    resource.Codec
	started  bool
	startMux sync.Mutex
}

//nolint:revive,staticcheck,gocritic
func (w *WatchResponse) start() {
	for {
		select {
		case evt := <-w.watch.ResultChan():
			if evt.Object == nil {
				if logging.DefaultLogger != nil {
					logging.DefaultLogger.Warn("Received nil object in watch event")
				}
				break
			}
			var obj resource.Object
			if cast, ok := evt.Object.(resource.Object); ok {
				obj = cast
			} else if cast, ok := evt.Object.(intoObject); ok {
				obj = w.ex.Copy()
				err := cast.Into(obj, w.codec)
				if err != nil {
					// TODO: hmm
					break
				}
			} else if cast, ok := evt.Object.(wrappedObject); ok {
				obj = cast.ResourceObject()
			} else {
				// TODO: hmm
				if logging.DefaultLogger != nil {
					logging.DefaultLogger.Error(
						"Unable to parse watch event object, does not implement resource.Object or have Into() or ResourceObject(). Please check your NegotiatedSerializer.",
						"groupVersionKind", evt.Object.GetObjectKind().GroupVersionKind().String())
				}
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
	w.startMux.Lock()
	defer w.startMux.Unlock()
	w.stopCh <- struct{}{}
	close(w.ch)
	w.watch.Stop()
	w.started = false
}

// WatchEvents returns a channel that receives watch events.
// All calls to this method will return the same channel.
// This channel will stop receiving events if KubernetesWatch() is called, as that halts the event translation process.
// If Stop() is called, ths channel is closed.
func (w *WatchResponse) WatchEvents() <-chan resource.WatchEvent {
	w.startMux.Lock()
	defer w.startMux.Unlock()
	if !w.started {
		// Start the translation buffer
		go w.start()
		w.started = true
	}
	return w.ch
}

// KubernetesWatch returns the underlying watch.Interface.
// Calling this method will shut down the translation channel between the watch.Interface and ResultChan().
// Using both KubernetesWatch() and ResultChan() simultaneously is not supported, and may result in undefined behavior.
func (w *WatchResponse) KubernetesWatch() watch.Interface {
	w.startMux.Lock()
	defer w.startMux.Unlock()
	// Stop the internal channel with the translation layer
	if w.started {
		w.stopCh <- struct{}{}
		w.started = false
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
//
//nolint:govet
func parseKubernetesError(responseBytes []byte, statusCode int, err error) error {
	if err != nil {
		statusErr := &k8serrors.StatusError{}
		if errors.As(err, &statusErr) {
			if statusCode == 0 || (statusErr.ErrStatus.Code > 0 && statusCode != int(statusErr.ErrStatus.Code)) {
				return NewServerResponseError(statusErr, int(statusErr.ErrStatus.Code))
			}
			return NewServerResponseError(statusErr, statusCode)
		}
	}
	if len(responseBytes) > 0 {
		parsed := k8sErrBody{}
		// If we can parse the response body, use the error contained there instead, because it's clearer
		if e := json.Unmarshal(responseBytes, &parsed); e == nil {
			err = errors.New(parsed.Message)
			if statusCode == 0 && parsed.Code > 0 {
				statusCode = parsed.Code
			}
		}
	}
	// HTTP error?
	if statusCode >= 300 {
		return NewServerResponseError(err, statusCode)
	}
	return err
}

func addLabels(obj resource.Object, labels map[string]string) {
	l := obj.GetLabels()
	if l == nil {
		l = make(map[string]string)
	}
	for k, v := range labels {
		l[k] = v
	}
	obj.SetLabels(l)
}
