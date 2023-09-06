package simple

import (
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestPlugin_HandleCRUDL(t *testing.T) {
	p := Plugin{}
	s, _ := resource.NewTypedStore[*resource.SimpleObject[string]](nil, nil)
	p.HandleCRUDL(nil, NewJSONResourceHandler[string, *resource.SimpleObject[string]](nil, &TestConverter{}, s), "")
}

type TestConverter struct {
}

func (t *TestConverter) ToAPI(r *resource.SimpleObject[string]) (string, error) {
	return r.Spec, nil
}
func (t *TestConverter) ToStore(s string) (*resource.SimpleObject[string], error) {
	return &resource.SimpleObject[string]{
		Spec: s,
	}, nil
}
