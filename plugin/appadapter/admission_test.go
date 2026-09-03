package appadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
)

func testKind() resource.Kind {
	return resource.Kind{
		Schema: resource.NewSimpleSchema(
			"test.grafana.app", "v1alpha1",
			&resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{},
			resource.WithKind("Foo"),
		),
		Codecs: map[resource.KindEncoding]resource.Codec{
			resource.KindEncodingJSON: resource.NewJSONCodec(),
		},
	}
}

func marshalFoo(t *testing.T, name, spec string) []byte {
	t.Helper()
	obj := &resource.TypedSpecObject[string]{Spec: spec}
	obj.Name = name
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal object: %v", err)
	}
	return b
}

// admissionFakeApp is a minimal app.App used to exercise AdmissionReview.
type admissionFakeApp struct {
	fakeApp
	managedKinds []resource.Kind
	validate     func(ctx context.Context, req *app.AdmissionRequest) error
	mutate       func(ctx context.Context, req *app.AdmissionRequest) (*app.MutatingResponse, error)
}

func (a *admissionFakeApp) ManagedKinds() []resource.Kind { return a.managedKinds }

func (a *admissionFakeApp) Validate(ctx context.Context, req *app.AdmissionRequest) error {
	if a.validate == nil {
		return nil
	}
	return a.validate(ctx, req)
}

func (a *admissionFakeApp) Mutate(ctx context.Context, req *app.AdmissionRequest) (*app.MutatingResponse, error) {
	if a.mutate == nil {
		return nil, app.ErrNotImplemented
	}
	return a.mutate(ctx, req)
}

func newAdmissionReviewRequest(op pluginv3.AdmissionReviewRequest_Operation, objectBytes, oldObjectBytes []byte) *pluginv3.AdmissionReviewRequest {
	kind := &pluginv3.GroupVersionKind{}
	kind.SetGroup("test.grafana.app")
	kind.SetVersion("v1alpha1")
	kind.SetKind("Foo")

	req := &pluginv3.AdmissionReviewRequest{}
	req.SetOperation(op)
	req.SetKind(kind)
	req.SetObjectBytes(objectBytes)
	req.SetOldObjectBytes(oldObjectBytes)
	return req
}

func TestAdmissionAdapter_AdmissionReview(t *testing.T) {
	t.Run("allows and decodes the object when validation and mutation pass", func(t *testing.T) {
		var gotReq *app.AdmissionRequest
		a := NewAdmissionAdapter(&admissionFakeApp{
			managedKinds: []resource.Kind{testKind()},
			validate: func(_ context.Context, req *app.AdmissionRequest) error {
				gotReq = req
				return nil
			},
		})

		objBytes := marshalFoo(t, "foo1", "hello")
		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_CREATE, objBytes, nil)

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if !rsp.GetAllowed() {
			t.Fatalf("expected allowed=true, got response: %+v", rsp)
		}
		if len(rsp.GetObjectBytes()) != 0 {
			t.Fatalf("expected no object_bytes when mutation is unimplemented, got %s", rsp.GetObjectBytes())
		}

		if gotReq == nil {
			t.Fatal("expected app.Validate to be invoked")
		}
		if gotReq.Action != resource.AdmissionActionCreate {
			t.Fatalf("unexpected action: %v", gotReq.Action)
		}
		obj, ok := gotReq.Object.(*resource.TypedSpecObject[string])
		if !ok {
			t.Fatalf("expected decoded object type *TypedSpecObject[string], got %T", gotReq.Object)
		}
		if obj.Name != "foo1" || obj.Spec != "hello" {
			t.Fatalf("unexpected decoded object: %+v", obj)
		}
	})

	t.Run("denies when Validate returns an error", func(t *testing.T) {
		a := NewAdmissionAdapter(&admissionFakeApp{
			managedKinds: []resource.Kind{testKind()},
			validate: func(context.Context, *app.AdmissionRequest) error {
				return errors.New("spec.dummy must not be forbidden")
			},
		})

		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_CREATE, marshalFoo(t, "foo1", "forbidden"), nil)

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if rsp.GetAllowed() {
			t.Fatal("expected allowed=false")
		}
		if rsp.GetError() == nil || rsp.GetError().GetMessage() != "spec.dummy must not be forbidden" {
			t.Fatalf("unexpected error status: %+v", rsp.GetError())
		}
	})

	t.Run("returns mutated object bytes when Mutate updates the object", func(t *testing.T) {
		a := NewAdmissionAdapter(&admissionFakeApp{
			managedKinds: []resource.Kind{testKind()},
			mutate: func(_ context.Context, req *app.AdmissionRequest) (*app.MutatingResponse, error) {
				updated := req.Object.(*resource.TypedSpecObject[string])
				updated.Spec = "mutated"
				return &app.MutatingResponse{UpdatedObject: updated}, nil
			},
		})

		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_UPDATE, marshalFoo(t, "foo1", "hello"), marshalFoo(t, "foo1", "hello"))

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if !rsp.GetAllowed() {
			t.Fatalf("expected allowed=true, got response: %+v", rsp)
		}

		var mutated resource.TypedSpecObject[string]
		if err := json.Unmarshal(rsp.GetObjectBytes(), &mutated); err != nil {
			t.Fatalf("unmarshal mutated object: %v", err)
		}
		if mutated.Spec != "mutated" {
			t.Fatalf("unexpected mutated spec: %q", mutated.Spec)
		}
	})

	t.Run("denies with the managed kinds when the kind is not managed", func(t *testing.T) {
		other := resource.Kind{
			Schema: resource.NewSimpleSchema(
				"other.grafana.app", "v1",
				&resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{},
				resource.WithKind("Bar"),
			),
			Codecs: map[resource.KindEncoding]resource.Codec{
				resource.KindEncodingJSON: resource.NewJSONCodec(),
			},
		}
		a := NewAdmissionAdapter(&admissionFakeApp{managedKinds: []resource.Kind{other}})

		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_CREATE, nil, nil)

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if rsp.GetAllowed() {
			t.Fatal("expected allowed=false for an unmanaged kind")
		}
		status := rsp.GetError()
		if status == nil {
			t.Fatal("expected an error status")
		}
		if status.GetCode() != http.StatusServiceUnavailable {
			t.Fatalf("unexpected code: %d", status.GetCode())
		}
		if status.GetReason() != string(metav1.StatusReasonServiceUnavailable) {
			t.Fatalf("unexpected reason: %q", status.GetReason())
		}
		details := status.GetDetails()
		if details == nil {
			t.Fatal("expected status details")
		}
		if details.GetGroup() != "test.grafana.app" || details.GetKind() != "Foo" {
			t.Fatalf("unexpected details: %+v", details)
		}
		causes := details.GetCauses()
		if len(causes) != 2 {
			t.Fatalf("expected 2 causes, got %d", len(causes))
		}
		if !strings.Contains(causes[1].GetReason(), "other.grafana.app/v1, Kind=Bar") {
			t.Fatalf("expected the managed kinds to be listed, got %q", causes[1].GetReason())
		}
	})

	t.Run("validates the mutated object and returns its bytes", func(t *testing.T) {
		var validated string
		a := NewAdmissionAdapter(&admissionFakeApp{
			managedKinds: []resource.Kind{testKind()},
			mutate: func(_ context.Context, req *app.AdmissionRequest) (*app.MutatingResponse, error) {
				updated := req.Object.(*resource.TypedSpecObject[string])
				updated.Spec = "mutated"
				return &app.MutatingResponse{UpdatedObject: updated}, nil
			},
			validate: func(_ context.Context, req *app.AdmissionRequest) error {
				validated = req.Object.(*resource.TypedSpecObject[string]).Spec
				return nil
			},
		})

		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_CREATE, marshalFoo(t, "foo1", "hello"), nil)

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if !rsp.GetAllowed() {
			t.Fatalf("expected allowed=true, got response: %+v", rsp)
		}
		if validated != "mutated" {
			t.Fatalf("expected Validate to see the mutated object, got spec %q", validated)
		}
	})

	t.Run("denies with structured details for an APIStatus error", func(t *testing.T) {
		a := NewAdmissionAdapter(&admissionFakeApp{
			managedKinds: []resource.Kind{testKind()},
			validate: func(context.Context, *app.AdmissionRequest) error {
				return apierrors.NewInvalid(
					schema.GroupKind{Group: "test.grafana.app", Kind: "Foo"},
					"foo1",
					field.ErrorList{field.Required(field.NewPath("spec"), "spec is required")},
				)
			},
		})

		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_CREATE, marshalFoo(t, "foo1", "hello"), nil)

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if rsp.GetAllowed() {
			t.Fatal("expected allowed=false")
		}
		status := rsp.GetError()
		if status.GetReason() != string(metav1.StatusReasonInvalid) {
			t.Fatalf("unexpected reason: %q", status.GetReason())
		}
		if status.GetCode() != http.StatusUnprocessableEntity {
			t.Fatalf("unexpected code: %d", status.GetCode())
		}
		details := status.GetDetails()
		if details == nil || details.GetName() != "foo1" || details.GetKind() != "Foo" {
			t.Fatalf("unexpected details: %+v", details)
		}
		if len(details.GetCauses()) != 1 {
			t.Fatalf("expected 1 cause, got %+v", details.GetCauses())
		}
		if got := details.GetCauses()[0]; got.GetField() != "spec" || got.GetReason() != string(metav1.CauseTypeFieldValueRequired) {
			t.Fatalf("unexpected cause: %+v", got)
		}
	})

	t.Run("treats a delete with no object bytes as a nil object", func(t *testing.T) {
		var gotReq *app.AdmissionRequest
		a := NewAdmissionAdapter(&admissionFakeApp{
			managedKinds: []resource.Kind{testKind()},
			validate: func(_ context.Context, req *app.AdmissionRequest) error {
				gotReq = req
				return nil
			},
		})

		req := newAdmissionReviewRequest(pluginv3.AdmissionReviewRequest_OPERATION_DELETE, nil, marshalFoo(t, "foo1", "hello"))

		rsp, err := a.AdmissionReview(context.Background(), req)
		if err != nil {
			t.Fatalf("AdmissionReview returned error: %v", err)
		}
		if !rsp.GetAllowed() {
			t.Fatalf("expected allowed=true, got response: %+v", rsp)
		}
		if gotReq.Object != nil {
			t.Fatalf("expected nil Object for a delete with no object bytes, got %+v", gotReq.Object)
		}
		if gotReq.OldObject == nil {
			t.Fatal("expected OldObject to be decoded")
		}
	})
}
