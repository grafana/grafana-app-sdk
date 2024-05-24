package apiserver

import (
	"bytes"
	"fmt"

	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceConverter is an interface which describes a type which can convert an object to and from its internal representation.
// Kubernetes API Server kind version conversion will always convert to and from the internal version when converting between versions,
// so converting from version v1 to v2 will go v1 -> internal -> v2, calling
// ToInternal(v1Obj, internalObj) then FromInternal(internalObj, v2Obj)
type ResourceConverter interface {
	FromInternal(src any, dst any) error
	ToInternal(src any, dst any) error
}

// TypedResourceConverter is an implementation of ResourceConverter which allows the user to work with typed inputs
type TypedResourceConverter[InternalType, ResourceType resource.Object] struct {
	FromInternalFunc func(src InternalType, dst ResourceType) error
	ToInternalFunc   func(src ResourceType, dst InternalType) error
}

// FromInternal attempts to convert src into InternalType and dst into ResourceType (returning an error if either of
// these operations fail), and then calls FromInternalFunc using the converted types.
// If FromInternalFunc is nil, it will call DefaultConversionFunc instead.
func (t *TypedResourceConverter[InternalType, ResourceType]) FromInternal(src, dst any) error {
	s, ok := src.(InternalType)
	if !ok {
		return fmt.Errorf("src is not of type InternalType")
	}
	d, ok := dst.(ResourceType)
	if !ok {
		return fmt.Errorf("dst is not of type ResourceType")
	}
	if t.FromInternalFunc == nil {
		return DefaultConversionFunc(s, d)
	}
	return t.FromInternalFunc(s, d)
}

// ToInternal attempts to convert src into ResourceType and dst into InternalType (returning an error if either of
// these operations fail), and then calls ToInternalFunc using the converted types.
// If ToInternalFunc is nil, it will call DefaultConversionFunc instead.
func (t *TypedResourceConverter[InternalType, ResourceType]) ToInternal(src, dst any) error {
	s, ok := src.(ResourceType)
	if !ok {
		return fmt.Errorf("src is not of type ResourceType")
	}
	d, ok := dst.(InternalType)
	if !ok {
		return fmt.Errorf("dst is not of type InternalType")
	}
	if t.FromInternalFunc == nil {
		return DefaultConversionFunc(s, d)
	}
	return t.ToInternalFunc(s, d)
}

var codec = resource.NewJSONCodec()

// DefaultConversionFunc is a default conversion function which translates src into dst by using a resource.JSONCodec
// to write src to JSON, then read the JSON bytes into dst.
func DefaultConversionFunc(src, dst resource.Object) error {
	buf := bytes.Buffer{}
	err := codec.Write(&buf, src)
	if err != nil {
		return err
	}
	err = codec.Read(bytes.NewReader(buf.Bytes()), dst)
	if err != nil {
		return err
	}
	return nil
}

// GenericObjectConverter implements ResourceConverter and calls DefaultConversionFunc for its conversion process.
// It uses the GVK value to set GroupVersionKind when converting.
type GenericObjectConverter struct {
	GVK schema.GroupVersionKind
}

// FromInternal attempts to convert src and dst into resource.Object, returning an error if this fails,
// then calls DefaultConversionFunc on the converted src and dst. It sets the GroupVersionKind on dst
// to the value of GenericObjectConverter.GVK.
func (g *GenericObjectConverter) FromInternal(src, dst any) error {
	a, ok := src.(resource.Object)
	if !ok {
		return fmt.Errorf("src does not implement resource.Object")
	}
	b, ok := dst.(resource.Object)
	if !ok {
		return fmt.Errorf("dst does not implement resource.Object")
	}
	err := DefaultConversionFunc(a, b)
	if err != nil {
		return err
	}
	b.SetGroupVersionKind(g.GVK)
	return nil
}

// ToInternal attempts to convert src and dst into resource.Object, returning an error if this fails,
// then calls DefaultConversionFunc on the converted src and dst. It sets the Group and Kind on dst
// to the value of GenericObjectConverter.GVK, and Version to runtime.APIVersionInternal.
func (g *GenericObjectConverter) ToInternal(src, dst any) error {
	a, ok := src.(resource.Object)
	if !ok {
		return fmt.Errorf("src does not implement resource.Object")
	}
	b, ok := dst.(resource.Object)
	if !ok {
		return fmt.Errorf("dst does not implement resource.Object")
	}
	err := DefaultConversionFunc(a, b)
	if err != nil {
		return err
	}
	b.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   g.GVK.Group,
		Version: runtime.APIVersionInternal,
		Kind:    g.GVK.Kind,
	})
	return nil
}
