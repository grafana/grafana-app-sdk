// This example shows how to read and write Grafana platform annotations (folder placement, manager
// ownership, and provisioning source) on a resource.Object using the SDK's exported constants and
// accessors, instead of hard-coding annotation strings.
//
// Run with: go run ./examples/resource/annotations
package main

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

func main() {
	// Any resource.Object works here; UntypedObject keeps the example dependency-free.
	obj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-dashboard",
			Namespace: "default",
		},
	}

	// --- Folder placement -------------------------------------------------------------------------
	resource.SetFolder(obj, "team-a-dashboards")
	_, _ = fmt.Printf("folder: %q\n", resource.GetFolder(obj))

	// --- Manager ownership ------------------------------------------------------------------------
	// Mark the resource as managed by a repo (e.g. provisioned via GitOps) and read-only.
	resource.SetManagerProperties(obj, resource.ManagerProperties{
		Kind:        resource.ManagerKindRepo,
		Identity:    "github.com/acme/dashboards",
		AllowsEdits: false,
	})

	if mgr, ok := resource.GetManagerProperties(obj); ok {
		_, _ = fmt.Printf("managed by %s (%s), edits allowed: %t\n", mgr.Kind, mgr.Identity, mgr.AllowsEdits)
		if mgr.Kind == resource.ManagerKindRepo && !mgr.AllowsEdits {
			_, _ = fmt.Println("-> resource is owned by a repo and read-only; an app should skip in-place edits")
		}
	}

	// --- Provisioning source ----------------------------------------------------------------------
	resource.SetSourceProperties(obj, resource.SourceProperties{
		Path:            "dashboards/my-dashboard.json",
		Checksum:        "9f2c0b1",
		TimestampMillis: 1718000000000,
	})

	if src, ok := resource.GetSourceProperties(obj); ok {
		_, _ = fmt.Printf("provisioned from path=%q checksum=%q\n", src.Path, src.Checksum)
	}

	// The accessors above are just typed wrappers over annotations keyed by the exported constants,
	// so the raw keys are always available too:
	_, _ = fmt.Println("\nraw annotations:")
	for _, key := range []string{
		resource.AnnoKeyFolder,
		resource.AnnoKeyManagerKind,
		resource.AnnoKeyManagerIdentity,
		resource.AnnoKeySourcePath,
	} {
		_, _ = fmt.Printf("  %s = %q\n", key, obj.GetAnnotations()[key])
	}
}
