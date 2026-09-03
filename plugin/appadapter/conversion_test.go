package appadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// conversionFakeApp is a minimal app.App used to exercise ConvertObjects.
type conversionFakeApp struct {
	fakeApp
	convert func(ctx context.Context, req app.ConversionRequest) (*app.RawObject, error)
}

func (a *conversionFakeApp) Convert(ctx context.Context, req app.ConversionRequest) (*app.RawObject, error) {
	return a.convert(ctx, req)
}

func newConvertGVK(group, version, kind string) *pluginv3.GroupVersionKind {
	gvk := &pluginv3.GroupVersionKind{}
	gvk.SetGroup(group)
	gvk.SetVersion(version)
	gvk.SetKind(kind)
	return gvk
}

func TestConversionAdapter_ConvertObjects(t *testing.T) {
	t.Run("converts each object and preserves order", func(t *testing.T) {
		var gotReqs []app.ConversionRequest
		a := NewConversionAdapter(&conversionFakeApp{
			convert: func(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
				gotReqs = append(gotReqs, req)
				return &app.RawObject{Raw: append([]byte("converted:"), req.Raw.Raw...)}, nil
			},
		})

		req := &pluginv3.ConvertObjectsRequest{}
		req.SetUid("uid-1")
		req.SetTargetVersion("v2")

		obj1 := &pluginv3.ConvertObjectsRequest_Object{}
		obj1.SetGvk(newConvertGVK("test.grafana.app", "v1", "Foo"))
		obj1.SetRaw([]byte("a"))

		obj2 := &pluginv3.ConvertObjectsRequest_Object{}
		obj2.SetGvk(newConvertGVK("test.grafana.app", "v1", "Foo"))
		obj2.SetRaw([]byte("b"))

		req.SetObjects([]*pluginv3.ConvertObjectsRequest_Object{obj1, obj2})

		rsp, err := a.ConvertObjects(context.Background(), req)
		if err != nil {
			t.Fatalf("ConvertObjects returned error: %v", err)
		}
		if rsp.GetUid() != "uid-1" {
			t.Fatalf("unexpected uid: %q", rsp.GetUid())
		}
		if rsp.GetError() != nil {
			t.Fatalf("expected no error status, got %+v", rsp.GetError())
		}

		converted := rsp.GetConverted()
		if len(converted) != 2 {
			t.Fatalf("expected 2 converted objects, got %d", len(converted))
		}
		if string(converted[0].GetRaw()) != "converted:a" || string(converted[1].GetRaw()) != "converted:b" {
			t.Fatalf("unexpected converted objects: %q, %q", converted[0].GetRaw(), converted[1].GetRaw())
		}

		if len(gotReqs) != 2 {
			t.Fatalf("expected 2 calls to Convert, got %d", len(gotReqs))
		}
		want := schema.GroupVersionKind{Group: "test.grafana.app", Version: "v1", Kind: "Foo"}
		wantTarget := schema.GroupVersionKind{Group: "test.grafana.app", Version: "v2", Kind: "Foo"}
		for i, r := range gotReqs {
			if r.SourceGVK != want {
				t.Fatalf("object %d: unexpected source GVK: %+v", i, r.SourceGVK)
			}
			if r.TargetGVK != wantTarget {
				t.Fatalf("object %d: unexpected target GVK: %+v", i, r.TargetGVK)
			}
		}
	})

	t.Run("stops at the first conversion error and reports it", func(t *testing.T) {
		calls := 0
		a := NewConversionAdapter(&conversionFakeApp{
			convert: func(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
				calls++
				if string(req.Raw.Raw) == "bad" {
					return nil, errors.New("cannot convert bad object")
				}
				return &app.RawObject{Raw: req.Raw.Raw}, nil
			},
		})

		req := &pluginv3.ConvertObjectsRequest{}
		req.SetUid("uid-2")
		req.SetTargetVersion("v2")

		good := &pluginv3.ConvertObjectsRequest_Object{}
		good.SetGvk(newConvertGVK("test.grafana.app", "v1", "Foo"))
		good.SetRaw([]byte("good"))

		bad := &pluginv3.ConvertObjectsRequest_Object{}
		bad.SetGvk(newConvertGVK("test.grafana.app", "v1", "Foo"))
		bad.SetRaw([]byte("bad"))

		req.SetObjects([]*pluginv3.ConvertObjectsRequest_Object{good, bad})

		rsp, err := a.ConvertObjects(context.Background(), req)
		if err != nil {
			t.Fatalf("ConvertObjects returned error: %v", err)
		}
		if rsp.GetError() == nil || rsp.GetError().GetMessage() != "cannot convert bad object" {
			t.Fatalf("unexpected error status: %+v", rsp.GetError())
		}
		if len(rsp.GetConverted()) != 0 {
			t.Fatalf("expected no converted objects on failure, got %d", len(rsp.GetConverted()))
		}
		if calls != 2 {
			t.Fatalf("expected Convert to be called for both objects up to and including the failure, got %d calls", calls)
		}
	})

	t.Run("handles an empty object list", func(t *testing.T) {
		a := NewConversionAdapter(&conversionFakeApp{
			convert: func(context.Context, app.ConversionRequest) (*app.RawObject, error) {
				t.Fatal("Convert should not be called")
				return nil, nil
			},
		})

		req := &pluginv3.ConvertObjectsRequest{}
		req.SetUid("uid-3")

		rsp, err := a.ConvertObjects(context.Background(), req)
		if err != nil {
			t.Fatalf("ConvertObjects returned error: %v", err)
		}
		if rsp.GetUid() != "uid-3" {
			t.Fatalf("unexpected uid: %q", rsp.GetUid())
		}
		if len(rsp.GetConverted()) != 0 {
			t.Fatalf("expected no converted objects, got %d", len(rsp.GetConverted()))
		}
	})
}
