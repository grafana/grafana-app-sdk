package k8s

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

// NewClientRegistry returns a new ClientRegistry which will make Client structs using the provided rest.Config
func NewClientRegistry(kubeCconfig rest.Config, clientConfig ClientConfig) *ClientRegistry {
	kubeCconfig.NegotiatedSerializer = &GenericNegotiatedSerializer{}
	kubeCconfig.UserAgent = rest.DefaultKubernetesUserAgent()

	return &ClientRegistry{
		clients:      make(map[schema.GroupVersion]rest.Interface),
		cfg:          kubeCconfig,
		clientConfig: clientConfig,
	}
}

// ClientRegistry implements resource.ClientGenerator, and keeps a cache of kubernetes clients based on
// GroupVersion (the largest unit a kubernetes rest.RESTClient can work with).
type ClientRegistry struct {
	clients      map[schema.GroupVersion]rest.Interface
	cfg          rest.Config
	clientConfig ClientConfig
	mutex        sync.Mutex
}

// ClientFor returns a Client with the underlying rest.Interface being a cached one for the Schema's GroupVersion.
// If no such client is cached, it creates a new one with the stored config.
func (c *ClientRegistry) ClientFor(sch resource.Schema) (resource.Client, error) {
	client, err := c.getClient(sch)
	if err != nil {
		return nil, err
	}
	return &Client{
		client: &groupVersionClient{
			client:  client,
			version: sch.Version(),
		},
		schema: sch,
		config: c.clientConfig,
	}, nil
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
