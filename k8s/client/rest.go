// The code in this file is copied from k8s.io/client-go/rest/client.go.
// It contains modifications to the original code to allow for a custom negotiator to be passed in config.

/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

// RESTConfig is a RESTConfig that allows for a custom negotiator to be passed in config.
type RESTConfig struct {
	rest.Config
	Negotiator runtime.ClientNegotiator
}

// RESTClientFor returns a RESTClient that satisfies the requested attributes on a client Config
// object. Note that a RESTClient may require fields that are optional when initializing a Client.
// A RESTClient created by this method is generic - it expects to operate on an API that follows
// the Kubernetes conventions, but may not be the Kubernetes API.
// RESTClientFor is equivalent to calling RESTClientForConfigAndClient(config, httpClient),
// where httpClient was generated with HTTPClientFor(config).
func RESTClientFor(config *RESTConfig) (*rest.RESTClient, error) {
	if config.GroupVersion == nil {
		return nil, fmt.Errorf("GroupVersion is required when initializing a RESTClient")
	}
	if config.Negotiator == nil {
		return nil, fmt.Errorf("Negotiator is required when initializing a RESTClient")
	}

	httpClient, err := rest.HTTPClientFor(&config.Config)
	if err != nil {
		return nil, err
	}

	baseURL, versionedAPIPath, err := rest.DefaultServerUrlFor(&config.Config)
	if err != nil {
		return nil, err
	}

	rateLimiter := config.RateLimiter
	if rateLimiter == nil {
		qps := config.QPS
		if config.QPS == 0.0 {
			qps = rest.DefaultQPS
		}
		burst := config.Burst
		if config.Burst == 0 {
			burst = rest.DefaultBurst
		}
		if qps > 0 {
			rateLimiter = flowcontrol.NewTokenBucketRateLimiter(qps, burst)
		}
	}

	var gv schema.GroupVersion
	if config.GroupVersion != nil {
		gv = *config.GroupVersion
	}

	if config.Negotiator == nil {
		config.Negotiator = runtime.NewClientNegotiator(config.NegotiatedSerializer, gv)
	}

	clientContent := rest.ClientContentConfig{
		AcceptContentTypes: config.AcceptContentTypes,
		ContentType:        config.ContentType,
		GroupVersion:       gv,
		Negotiator:         config.Negotiator,
	}

	return rest.NewRESTClient(baseURL, versionedAPIPath, clientContent, rateLimiter, httpClient)
}
