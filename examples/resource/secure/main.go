// Run with: go run ./examples/resource/secure
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	obj := &resource.TypedSpecObject[map[string]string]{Spec: map[string]string{"endpoint": "https://example.com"}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "example.grafana.app", Version: "v1", Kind: "Connection"})
	obj.SetName("example")
	obj.Secure = resource.InlineSecureValues{
		"apiKey": {Create: resource.NewSecretValue("example-only-value")},
		"token":  {Name: "existing-secret-reference"},
	}
	fmt.Println("Direct JSON encoding (redacted):")
	if err := json.NewEncoder(os.Stdout).Encode(obj); err != nil {
		return err
	}
	fmt.Println("JSONCodec output (plaintext create values):")
	if err := (&resource.JSONCodec{}).Write(os.Stdout, obj); err != nil {
		return err
	}
	fmt.Println("OpenAPI secure property:")
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	versionSchema, err := app.VersionSchemaFromMap(map[string]any{"spec": map[string]any{"type": "object"}}, "Connection")
	if err != nil {
		return err
	}
	definitions, err := versionSchema.AsKubeOpenAPI(obj.GroupVersionKind(), spec.MustCreateRef, "example",
		app.ManifestVersionKindSecureValue{Key: "apiKey", Description: "API key from the service"},
		app.ManifestVersionKindSecureValue{Key: "token"},
	)
	if err != nil {
		return err
	}
	return encoder.Encode(definitions["example.Connection"].Schema.Properties["secure"])
}
