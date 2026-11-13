package operator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

// NewIdentityObject returns the kind's zero value carrying one object's identity and nothing else. It is what an
// identity payload delivers, so a consumer which has to build the same shape — a test, or a lister reading a
// server-specific projection — produces what the informer does.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewIdentityObject(kind resource.Kind, namespace, name, resourceVersion string) resource.Object {
	obj := kind.ZeroValue()
	obj.SetGroupVersionKind(kind.GroupVersionKind())
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetResourceVersion(resourceVersion)
	return obj
}

// newIdentityObjectFrom projects an object onto NewIdentityObject, retaining the metadata a change bus can announce
// with a live event. The metadata is kept because live events carry it: dropping it would make a re-list deliver
// less than the live stream, and would leave a selector with nothing to match on.
func newIdentityObjectFrom(kind resource.Kind, obj metav1.Object) resource.Object {
	identity := NewIdentityObject(kind, obj.GetNamespace(), obj.GetName(), obj.GetResourceVersion())
	identity.SetLabels(obj.GetLabels())
	identity.SetAnnotations(obj.GetAnnotations())
	identity.SetFinalizers(obj.GetFinalizers())
	identity.SetGeneration(obj.GetGeneration())
	identity.SetCreationTimestamp(obj.GetCreationTimestamp())
	identity.SetDeletionTimestamp(obj.GetDeletionTimestamp())
	identity.SetUID(obj.GetUID())
	return identity
}
