package k8s

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

var _ resource.ClientGenerator = &ClientRegistry{}

// NewClientRegistry returns a new ClientRegistry which will make Client structs using the provided rest.Config
func NewClientRegistry(kubeCconfig rest.Config, clientConfig ClientConfig) *ClientRegistry {
	kubeCconfig.NegotiatedSerializer = &GenericNegotiatedSerializer{}
	kubeCconfig.UserAgent = rest.DefaultKubernetesUserAgent()

	return &ClientRegistry{
		clients:      make(map[schema.GroupVersion]rest.Interface),
		cfg:          kubeCconfig,
		clientConfig: clientConfig,
		requestDurations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       clientConfig.MetricsConfig.Namespace,
			Subsystem:                       "kubernetes_client",
			Name:                            "request_duration_seconds",
			Help:                            "Time (in seconds) spent serving HTTP requests.",
			Buckets:                         metrics.LatencyBuckets,
			NativeHistogramBucketFactor:     clientConfig.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber:  clientConfig.MetricsConfig.NativeHistogramMaxBucketNumber,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"status_code", "verb", "kind", "subresource"}),
		totalRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:      "requests_total",
			Subsystem: "kubernetes_client",
			Namespace: clientConfig.MetricsConfig.Namespace,
			Help:      "Total number of kubernetes requests",
		}, []string{"status_code", "verb", "kind", "subresource"}),
	}
}

// ClientRegistry implements resource.ClientGenerator, and keeps a cache of kubernetes clients based on
// GroupVersion (the largest unit a kubernetes rest.RESTClient can work with).
type ClientRegistry struct {
	clients          map[schema.GroupVersion]rest.Interface
	cfg              rest.Config
	clientConfig     ClientConfig
	mutex            sync.Mutex
	requestDurations *prometheus.HistogramVec
	totalRequests    *prometheus.CounterVec
}

// ClientFor returns a Client with the underlying rest.Interface being a cached one for the Schema's GroupVersion.
// If no such client is cached, it creates a new one with the stored config.
func (c *ClientRegistry) ClientFor(sch resource.Kind) (resource.Client, error) {
	codec := sch.Codec(resource.KindEncodingJSON)
	if codec == nil {
		return nil, fmt.Errorf("no codec for KindEncodingJSON")
	}
	client, err := c.getClient(sch)
	if err != nil {
		return nil, err
	}
	return &Client{
		client: &groupVersionClient{
			client:           client,
			version:          sch.Version(),
			config:           c.clientConfig,
			requestDurations: c.requestDurations,
			totalRequests:    c.totalRequests,
		},
		schema: sch,
		codec:  codec,
		config: c.clientConfig,
	}, nil
}

// PrometheusCollectors returns the prometheus metric collectors used by all clients generated by this ClientRegistry to allow for registration
func (c *ClientRegistry) PrometheusCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		c.totalRequests, c.requestDurations,
	}
}

func (c *ClientRegistry) getClient(sch resource.Schema) (rest.Interface, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	gv := schema.GroupVersion{
		Group:   sch.Group(),
		Version: sch.Version(),
	}

	if c, ok := c.clients[gv]; ok {
		return c, nil
	}

	ccfg := c.cfg
	ccfg.GroupVersion = &gv
	client, err := rest.RESTClientFor(&ccfg)
	if err != nil {
		return nil, err
	}
	c.clients[gv] = client
	return client, nil
}
