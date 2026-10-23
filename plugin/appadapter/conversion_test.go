package appadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
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
		req.SetTarget(newConvertGVK("test.grafana.app", "v2", "Foo"))

		obj1 := []byte(`{"apiVersion":"test.grafana.app/v1","kind":"Foo","metadata":{"name":"a"}}`)
		obj2 := []byte(`{"apiVersion":"test.grafana.app/v1beta1","kind":"Foo","metadata":{"name":"b"}}`)
		req.SetObjects([][]byte{obj1, obj2})

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
		if string(converted[0].GetRaw()) != "converted:"+string(obj1) || string(converted[1].GetRaw()) != "converted:"+string(obj2) {
			t.Fatalf("unexpected converted objects: %q, %q", converted[0].GetRaw(), converted[1].GetRaw())
		}

		if len(gotReqs) != 2 {
			t.Fatalf("expected 2 calls to Convert, got %d", len(gotReqs))
		}
		wantSources := []schema.GroupVersionKind{
			{Group: "test.grafana.app", Version: "v1", Kind: "Foo"},
			{Group: "test.grafana.app", Version: "v1beta1", Kind: "Foo"},
		}
		wantTarget := schema.GroupVersionKind{Group: "test.grafana.app", Version: "v2", Kind: "Foo"}
		for i, r := range gotReqs {
			if r.SourceGVK != wantSources[i] {
				t.Fatalf("object %d: unexpected source GVK: %+v", i, r.SourceGVK)
			}
			if r.TargetGVK != wantTarget {
				t.Fatalf("object %d: unexpected target GVK: %+v", i, r.TargetGVK)
			}
			require.Equal(t, req.GetObjects()[i], r.Raw.Raw)
			require.Equal(t, resource.KindEncodingJSON, r.Raw.Encoding)
		}
	})

	t.Run("stops at the first conversion error and reports it", func(t *testing.T) {
		good := []byte(`{"apiVersion":"test.grafana.app/v1","kind":"Foo","metadata":{"name":"good"}}`)
		bad := []byte(`{"apiVersion":"test.grafana.app/v1","kind":"Foo","metadata":{"name":"bad"}}`)
		calls := 0
		a := NewConversionAdapter(&conversionFakeApp{
			convert: func(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
				calls++
				if string(req.Raw.Raw) == string(bad) {
					return nil, errors.New("cannot convert bad object")
				}
				return &app.RawObject{Raw: req.Raw.Raw}, nil
			},
		})

		req := &pluginv3.ConvertObjectsRequest{}
		req.SetUid("uid-2")
		req.SetTarget(newConvertGVK("test.grafana.app", "v2", "Foo"))
		req.SetObjects([][]byte{good, bad, good})

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

	t.Run("parses a core API source independently of the target", func(t *testing.T) {
		raw := []byte(`{"apiVersion":"v1","kind":"ConfigMap"}`)
		a := NewConversionAdapter(&conversionFakeApp{
			convert: func(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
				require.Equal(t, schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, req.SourceGVK)
				require.Equal(t, schema.GroupVersionKind{Group: "test.grafana.app", Version: "v2", Kind: "Foo"}, req.TargetGVK)
				return &req.Raw, nil
			},
		})
		req := &pluginv3.ConvertObjectsRequest{}
		req.SetTarget(newConvertGVK("test.grafana.app", "v2", "Foo"))
		req.SetObjects([][]byte{raw})
		rsp, err := a.ConvertObjects(context.Background(), req)
		require.NoError(t, err)
		require.Nil(t, rsp.GetError())
		require.Len(t, rsp.GetConverted(), 1)
		require.Equal(t, raw, rsp.GetConverted()[0].GetRaw())
	})

	for _, tt := range []struct {
		name string
		raw  []byte
	}{
		{name: "malformed JSON", raw: []byte(`{"apiVersion":`)},
		{name: "invalid API version", raw: []byte(`{"apiVersion":"group/v1/invalid","kind":"Foo"}`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			a := NewConversionAdapter(&conversionFakeApp{
				convert: func(_ context.Context, req app.ConversionRequest) (*app.RawObject, error) {
					calls++
					return &req.Raw, nil
				},
			})
			good := []byte(`{"apiVersion":"test.grafana.app/v1","kind":"Foo"}`)
			req := &pluginv3.ConvertObjectsRequest{}
			req.SetUid("parse-error")
			req.SetTarget(newConvertGVK("test.grafana.app", "v2", "Foo"))
			req.SetObjects([][]byte{good, tt.raw, good})

			rsp, err := a.ConvertObjects(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, "parse-error", rsp.GetUid())
			require.NotNil(t, rsp.GetError())
			require.Equal(t, "Failure", rsp.GetError().GetStatus())
			require.NotEmpty(t, rsp.GetError().GetMessage())
			require.Empty(t, rsp.GetConverted())
			require.Equal(t, 1, calls, "parsing failure stops conversion and discards partial results")
		})
	}

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
