package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
)

func TestBuildClientGenerator(t *testing.T) {
	generator := BuildClientGenerator(rest.Config{Host: "https://example.com"})

	_, ok := generator.(*k8s.ClientRegistry)
	assert.True(t, ok, "expected a *k8s.ClientRegistry")
}
