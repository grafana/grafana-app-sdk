// Package openapiutil contains OpenAPI helpers shared by codegen packages.
package openapiutil

import (
	"encoding/json"

	"k8s.io/kube-openapi/pkg/spec3"
)

// MergePaths merges source into target at the path-field level: an operation in
// source replaces the same operation in target, while operations only present in
// target are retained. target is modified in place (and allocated if nil).
func MergePaths(target, source map[string]spec3.PathProps) (map[string]spec3.PathProps, error) {
	if len(source) == 0 {
		return target, nil
	}
	if target == nil {
		target = make(map[string]spec3.PathProps, len(source))
	}
	for path, props := range source {
		base, err := json.Marshal(target[path])
		if err != nil {
			return nil, err
		}
		override, err := json.Marshal(props)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(base, &fields); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(override, &fields); err != nil {
			return nil, err
		}
		merged, err := json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		var result spec3.PathProps
		if err := json.Unmarshal(merged, &result); err != nil {
			return nil, err
		}
		target[path] = result
	}
	return target, nil
}
