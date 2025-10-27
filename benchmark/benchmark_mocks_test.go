package benchmark_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Buffer pools to reduce allocations during benchmarks
var bufferPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putBuffer(buf *bytes.Buffer) {
	// Don't pool buffers larger than 64KB to avoid memory bloat
	if buf.Cap() > 64*1024 {
		return
	}
	bufferPool.Put(buf)
}

// mockRestClient creates a rest.Interface that uses a mock HTTP transport
// to return Kubernetes-formatted JSON responses without making real HTTP calls.
func newMockRestClient(kind resource.Kind, objects []resource.Object) (rest.Interface, error) {
	transport := &mockRoundTripper{
		objects: objects,
		kind:    kind,
	}

	config := &rest.Config{
		Host: "https://mock-api-server",
		ContentConfig: rest.ContentConfig{
			GroupVersion: &schema.GroupVersion{
				Group:   kind.Schema.Group(),
				Version: kind.Schema.Version(),
			},
			NegotiatedSerializer: &k8s.GenericNegotiatedSerializer{},
		},
		Transport: transport,
	}

	return rest.RESTClientFor(config)
}

// mockRoundTripper implements http.RoundTripper to intercept HTTP requests
// and return mock Kubernetes API responses.
type mockRoundTripper struct {
	objects []resource.Object
	kind    resource.Kind
	mu      sync.RWMutex
}

var _ http.RoundTripper = &mockRoundTripper{}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse URL to determine request type
	u := req.URL

	// Check if this is a watch request
	if u.Query().Get("watch") == "1" {
		return m.handleWatch(req)
	}

	// Handle LIST request
	if req.Method == http.MethodGet {
		return m.handleList(req)
	}

	// Other operations not needed for benchmarks
	return &http.Response{
		StatusCode: http.StatusNotImplemented,
		Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
		Header:     make(http.Header),
	}, nil
}

func (m *mockRoundTripper) handleList(req *http.Request) (*http.Response, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := req.URL.Query()

	// Parse pagination parameters
	startIdx := 0
	if continueToken := query.Get("continue"); continueToken != "" {
		var err error
		startIdx, err = strconv.Atoi(continueToken)
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"invalid continue token"}`))),
				Header:     make(http.Header),
			}, nil
		}
	}

	endIdx := len(m.objects)
	var continueToken string

	// Handle pagination with limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"invalid limit"}`))),
				Header:     make(http.Header),
			}, nil
		}
		endIdx = startIdx + limit
		if endIdx > len(m.objects) {
			endIdx = len(m.objects)
		}
		if endIdx < len(m.objects) {
			continueToken = strconv.Itoa(endIdx)
		}
	}

	// Get codec for marshaling objects
	codec := m.kind.Codecs[resource.KindEncodingJSON]

	// Marshal objects to JSON using the real codec
	items := make([]json.RawMessage, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		buf := getBuffer()
		if err := codec.Write(buf, m.objects[i]); err != nil {
			putBuffer(buf)
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(`{"error":"marshal error: %v"}`, err)))),
				Header:     make(http.Header),
			}, nil
		}
		// Copy buffer contents before returning to pool
		itemData := make([]byte, buf.Len())
		copy(itemData, buf.Bytes())
		items = append(items, json.RawMessage(itemData))
		putBuffer(buf)
	}

	// Create Kubernetes List response format
	listResp := map[string]interface{}{
		"apiVersion": m.kind.Schema.Group() + "/" + m.kind.Schema.Version(),
		"kind":       m.kind.Schema.Kind() + "List",
		"metadata": map[string]interface{}{
			"resourceVersion": "1000",
		},
		"items": items,
	}

	if continueToken != "" {
		listResp["metadata"].(map[string]interface{})["continue"] = continueToken
	}

	// Marshal the full response
	body, err := json.Marshal(listResp)
	if err != nil {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(`{"error":"response marshal error: %v"}`, err)))),
			Header:     make(http.Header),
		}, nil
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     headers,
	}, nil
}

func (m *mockRoundTripper) handleWatch(req *http.Request) (*http.Response, error) {
	query := req.URL.Query()
	useWatchList := query.Get("sendInitialEvents") == "true"

	// Create a pipe for streaming watch events
	pr, pw := io.Pipe()

	// Start goroutine to write watch events
	go func() {
		defer pw.Close()
		m.writeWatchEvents(pw, useWatchList)
	}()

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Transfer-Encoding", "chunked")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     headers,
	}, nil
}

func (m *mockRoundTripper) writeWatchEvents(w io.Writer, useWatchList bool) {
	m.mu.RLock()
	objects := m.objects
	m.mu.RUnlock()

	codec := m.kind.Codecs[resource.KindEncodingJSON]

	// Write ADDED events for all objects
	for _, obj := range objects {
		// Marshal object to JSON using codec
		buf := getBuffer()
		if err := codec.Write(buf, obj); err != nil {
			putBuffer(buf)
			continue
		}

		// Create watch event by directly embedding the JSON bytes
		// This avoids the unmarshal->marshal cycle
		// Format: {"type":"ADDED","object":<json-bytes>}\n
		if _, err := w.Write([]byte(`{"type":"ADDED","object":`)); err != nil {
			putBuffer(buf)
			continue
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			putBuffer(buf)
			continue
		}
		if _, err := w.Write([]byte("}\n")); err != nil {
			putBuffer(buf)
			continue
		}
		putBuffer(buf)
	}

	// Send BOOKMARK event for watch-list protocol
	if useWatchList && len(objects) > 0 {
		lastObj := objects[len(objects)-1]

		// Construct bookmark JSON directly to avoid map marshaling
		bookmarkJSON := fmt.Sprintf(
			`{"type":"BOOKMARK","object":{"apiVersion":"%s/%s","kind":"%s","metadata":{"resourceVersion":"%s","annotations":{"%s":"true"}}}}`,
			m.kind.Schema.Group(),
			m.kind.Schema.Version(),
			m.kind.Schema.Kind(),
			lastObj.GetResourceVersion(),
			metav1.InitialEventsAnnotationKey,
		)
		// Ignore write errors in benchmark mock - connection may be closed
		_, _ = w.Write([]byte(bookmarkJSON + "\n"))
	}

	// Keep connection open until context is done
	// In a real scenario, the request context would handle this
	// For benchmarks, we just need to emit the initial events
}

// mockClientGeneratorWithK8sClient implements resource.ClientGenerator using real k8s.Client
// instances with a mock REST transport. This exercises the full production code path including
// groupVersionClient, translation layer (rawToListWithParser), and codec operations.
type mockClientGeneratorWithK8sClient struct {
	kind    resource.Kind
	objects []resource.Object
}

func (m *mockClientGeneratorWithK8sClient) ClientFor(kind resource.Kind) (resource.Client, error) {
	// Create mock rest.Interface with HTTP mock transport
	restClient, err := newMockRestClient(m.kind, m.objects)
	if err != nil {
		return nil, err
	}

	// Create REAL k8s.Client using the production constructor
	// This will use the real groupVersionClient implementation
	return k8s.NewClientWithRESTInterface(
		restClient,
		m.kind,
		k8s.DefaultClientConfig(),
	)
}
