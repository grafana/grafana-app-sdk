package main

import (
	"os"

	"github.com/grafana/grafana-app-sdk/apiserver"
	cmd "github.com/grafana/grafana-app-sdk/cmd/apiserver"
	corev1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/core/v1"
	"k8s.io/component-base/cli"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func main() {
	// Create an API Server Resource for the
	r := apiserver.Resource{
		Kind:                  corev1.ExternalNameKind(),
		GetOpenAPIDefinitions: corev1.GetOpenAPIDefinitions,
		/*Subresources: []apiserver.SubresourceRoute{{
			Path:        "foo",
			OpenAPISpec: fooSubresourceOpenAPI,
			Handler: func(w http.ResponseWriter, r *http.Request, identifier resource.Identifier) {
				w.Write([]byte(`{"foo":"bar"}`))
			},
		}},*/
	}
	g := apiserver.ResourceGroup{
		Name:      r.Kind.Group(),
		Resources: []apiserver.Resource{r},
	}

	o := cmd.NewAPIServerOptions([]apiserver.ResourceGroup{g}, os.Stdout, os.Stderr)
	o.RecommendedOptions.Admission = nil
	o.RecommendedOptions.Authorization = nil
	o.RecommendedOptions.Authentication = nil
	o.RecommendedOptions.CoreAPI = nil

	ch := make(chan struct{})
	cmd := cmd.NewCommandStartAPIServer(o, ch)

	code := cli.Run(cmd)
	os.Exit(code)
}

func fooSubresourceOpenAPI(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"github.com/grafana/grafana-app-sdk/examples/apiserver/apis/core/v1.ExternalNameFoo": common.OpenAPIDefinition{
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Description: "ExternalNameFoo defines model for ExternalNameFoo.",
					Type:        []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": {
							SchemaProps: spec.SchemaProps{
								Default: "",
								Type:    []string{"string"},
								Format:  "",
							},
						},
					},
					Required: []string{"foo"},
				},
			},
		},
	}
}
