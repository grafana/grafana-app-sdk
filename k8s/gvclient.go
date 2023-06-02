package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	versionLabel = "grafana-app-sdk-resource-version"

	annotationPrefix = "grafana.com/"
)

// groupVersionClient is the underlying client both Client and SchemalessClient use.
// GroupVersion is the unit with which kubernetes rest.Interface clients exist, so at minimum,
// we require one rest.Interface for each unique GroupVersion.
type groupVersionClient struct {
	client  rest.Interface
	version string
	config  ClientConfig
}

func (g *groupVersionClient) get(ctx context.Context, identifier resource.Identifier, plural string,
	into resource.Object) error {
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	bytes, err := request.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return parseKubernetesError(bytes, sc, err)
	}
	return rawToObject(bytes, into)
}

func (g *groupVersionClient) getMetadata(ctx context.Context, identifier resource.Identifier, plural string) (
	*k8sObject, error) {
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	bytes, err := request.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return nil, parseKubernetesError(bytes, sc, err)
	}
	md := k8sObject{}
	err = json.Unmarshal(bytes, &md)
	if err != nil {
		return nil, err
	}
	return &md, nil
}

//nolint:unused
func (g *groupVersionClient) exists(ctx context.Context, identifier resource.Identifier, plural string) (
	bool, error) {
	sc := 0
	request := g.client.Get().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	err := request.Do(ctx).StatusCode(&sc).Error()
	if err != nil {
		// HTTP error?
		if sc == http.StatusNotFound {
			return false, nil
		}
		if sc > 0 {
			return false, NewServerResponseError(err, sc)
		}
		return false, err
	}
	return true, nil
}

func (g *groupVersionClient) create(ctx context.Context, plural string, obj resource.Object,
	into resource.Object) error {
	bytes, err := marshalJSON(obj, map[string]string{
		versionLabel: g.version,
	}, g.config)
	if err != nil {
		return err
	}

	sc := 0
	request := g.client.Post().Resource(plural).Body(bytes)
	if strings.TrimSpace(obj.StaticMetadata().Namespace) != "" {
		request = request.Namespace(obj.StaticMetadata().Namespace)
	}
	bytes, err = request.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return parseKubernetesError(bytes, sc, err)
	}
	return rawToObject(bytes, into)
}

func (g *groupVersionClient) update(ctx context.Context, plural string, obj resource.Object,
	into resource.Object, _ resource.UpdateOptions) error {
	bytes, err := marshalJSON(obj, map[string]string{
		versionLabel: g.version,
	}, g.config)
	if err != nil {
		return err
	}

	req := g.client.Put().Resource(plural).
		Name(obj.StaticMetadata().Name).Body(bytes)
	if strings.TrimSpace(obj.StaticMetadata().Namespace) != "" {
		req = req.Namespace(obj.StaticMetadata().Namespace)
	}
	sc := 0
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return parseKubernetesError(raw, sc, err)
	}
	return rawToObject(raw, into)
}

func (g *groupVersionClient) updateSubresource(ctx context.Context, plural, subresource string, obj resource.Object,
	into resource.Object, _ resource.UpdateOptions) error {
	bytes, err := marshalJSON(obj, map[string]string{
		versionLabel: g.version,
	}, g.config)
	if err != nil {
		return err
	}

	req := g.client.Put().Resource(plural).SubResource(subresource).
		Name(obj.StaticMetadata().Name).Body(bytes)
	if strings.TrimSpace(obj.StaticMetadata().Namespace) != "" {
		req = req.Namespace(obj.StaticMetadata().Namespace)
	}
	sc := 0
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return parseKubernetesError(raw, sc, err)
	}
	return rawToObject(raw, into)
}

//nolint:revive,unused
func (g *groupVersionClient) patch(ctx context.Context, identifier resource.Identifier, plural string,
	patch resource.PatchRequest, into resource.Object, _ resource.PatchOptions) error {
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
	raw, err := req.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return parseKubernetesError(raw, sc, err)
	}
	return rawToObject(raw, into)
}

func (g *groupVersionClient) delete(ctx context.Context, identifier resource.Identifier, plural string) error {
	sc := 0
	request := g.client.Delete().Resource(plural).Name(identifier.Name)
	if strings.TrimSpace(identifier.Namespace) != "" {
		request = request.Namespace(identifier.Namespace)
	}
	err := request.
		Do(ctx).StatusCode(&sc).Error()
	if err != nil && sc >= 300 {
		return NewServerResponseError(err, sc)
	}
	return err
}

func (g *groupVersionClient) list(ctx context.Context, namespace, plural string, into resource.ListObject,
	options resource.ListOptions, itemParser func([]byte) (resource.Object, error)) error {
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
	bytes, err := req.Do(ctx).StatusCode(&sc).Raw()
	if err != nil {
		return parseKubernetesError(bytes, sc, err)
	}
	return rawToListWithParser(bytes, into, itemParser)
}

//nolint:revive
func (g *groupVersionClient) watch(ctx context.Context, namespace, plural string,
	exampleObject resource.Object, options resource.WatchOptions) (*WatchResponse, error) {
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
		return nil, err
	}
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
