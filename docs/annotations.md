# Platform Annotations

Grafana's app platform stores a set of generic, platform-wide metadata about every resource in
Kubernetes annotations — who created it, which folder it lives in, whether a provisioning tool
manages it, and so on. The SDK exposes these keys (and supporting types/accessors) as stable
exported symbols in the `resource` package so that apps — **including those living outside the
`grafana/grafana` repo** — can read and write them through constants and helpers instead of
hard-coding strings.

The canonical source of truth for these keys is `grafana/grafana`'s
[`pkg/apimachinery/utils/meta.go`](https://github.com/grafana/grafana/blob/main/pkg/apimachinery/utils/meta.go)
and [`pkg/apimachinery/utils/manager.go`](https://github.com/grafana/grafana/blob/main/pkg/apimachinery/utils/manager.go).
A unit test (`resource/annotations_test.go`) asserts the SDK constant values match core, so consumers
can trust the SDK stays in sync with the platform.

## Prefix policy

| | Prefix | Spelling | Status |
|---|---|---|---|
| Canonical (core) | `grafana.app/` | `updatedTimestamp` | preferred |
| Legacy (older SDK) | `grafana.com/` | `updateTimestamp` | deprecated, still read |

New constants use the canonical `grafana.app/` prefix (`resource.CanonicalAnnotationPrefix`). The
three legacy SDK constants (`AnnotationCreatedBy`, `AnnotationUpdatedBy`, `AnnotationUpdateTimestamp`,
all on `resource.AnnotationPrefix` = `grafana.com/`) remain defined but are deprecated.

**Writers prefer the canonical keys; readers fall back to the legacy keys.** `GetCommonMetadata` reads
the `grafana.app/` keys first and falls back to the `grafana.com/` keys, so objects written by core and
by older SDK versions both read correctly. No migration of stored data is required.

## Exported constants

| Constant | Key |
|---|---|
| `resource.AnnoKeyCreatedBy` | `grafana.app/createdBy` |
| `resource.AnnoKeyUpdatedBy` | `grafana.app/updatedBy` |
| `resource.AnnoKeyUpdatedTimestamp` | `grafana.app/updatedTimestamp` |
| `resource.AnnoKeyFolder` | `grafana.app/folder` |
| `resource.AnnoKeyMessage` | `grafana.app/message` |
| `resource.AnnoKeyGrantPermissions` | `grafana.app/grant-permissions` |
| `resource.AnnoKeyManagerKind` | `grafana.app/managedBy` |
| `resource.AnnoKeyManagerIdentity` | `grafana.app/managerId` |
| `resource.AnnoKeyManagerAllowsEdits` | `grafana.app/managerAllowsEdits` |
| `resource.AnnoKeyManagerSuspended` | `grafana.app/managerSuspended` |
| `resource.AnnoKeySourcePath` | `grafana.app/sourcePath` |
| `resource.AnnoKeySourceChecksum` | `grafana.app/sourceChecksum` |
| `resource.AnnoKeySourceTimestamp` | `grafana.app/sourceTimestamp` |

Plus `resource.AnnoGrantPermissionsDefault` (`"default"`), the value to send with
`AnnoKeyGrantPermissions`.

## Supporting types & accessors

For the manager/source/folder contract, prefer the accessors over juggling individual keys — this is
where mirroring the *types* (not just the constants) pays off. All accessors operate on any
`metav1.Object`, which `resource.Object` satisfies.

- `resource.GetFolder(obj)` / `resource.SetFolder(obj, uid)` — folder placement.
- `resource.GetManagerProperties(obj)` / `resource.SetManagerProperties(obj, mgr)` — which tool, if any,
  manages the resource (`resource.ManagerProperties` + the `resource.ManagerKind` enum:
  `ManagerKindRepo`, `ManagerKindTerraform`, `ManagerKindKubectl`, `ManagerKindPlugin`,
  `ManagerKindGrafana`).
- `resource.GetSourceProperties(obj)` / `resource.SetSourceProperties(obj, src)` — provisioning source
  details (`resource.SourceProperties`: `Path`, `Checksum`, `TimestampMillis`).

`GetManagerProperties` and `GetSourceProperties` read the legacy `grafana.app/repo*` keys as a fallback
to match core's semantics.

## Usage

### Referencing keys instead of hard-coding strings

```go
import "github.com/grafana/grafana-app-sdk/resource"

// Before: brittle, easy to typo, prefix drifts out of sync with core.
folder := obj.GetAnnotations()["grafana.app/folder"]

// After: stable constant, kept in sync with grafana/grafana.
folder := obj.GetAnnotations()[resource.AnnoKeyFolder]
```

### Reading structured metadata via accessors

```go
// Folder placement.
folderUID := resource.GetFolder(obj)
resource.SetFolder(obj, "team-a-dashboards")

// Detect whether a resource is provisioned/managed, and by what.
if mgr, ok := resource.GetManagerProperties(obj); ok && mgr.Kind == resource.ManagerKindRepo && !mgr.AllowsEdits {
    // resource is owned by a repo and read-only — skip in-app edits
    return
}

// Provisioning source details.
if src, ok := resource.GetSourceProperties(obj); ok {
    log.Info("provisioned from", "path", src.Path, "checksum", src.Checksum)
}
```

### In a reconciler

The common real-world case — gate reconcile logic on ownership so you don't fight a GitOps/provisioning
tool:

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req operator.ReconcileRequest) (operator.ReconcileResult, error) {
    obj := req.Object

    // Don't mutate resources a manager owns and has locked.
    if mgr, ok := resource.GetManagerProperties(obj); ok && !mgr.AllowsEdits {
        return operator.ReconcileResult{}, nil
    }

    // Use folder placement to scope downstream work.
    if resource.GetFolder(obj) == "general" {
        // ...
    }
    return operator.ReconcileResult{}, nil
}
```

## The contract for consumers

- Exported constant **names** (`AnnoKeyFolder`, `ManagerKindRepo`, …) are part of the SDK's public API
  and won't change underneath consumers.
- The **key string values** track `grafana/grafana` (`grafana.app/…`); a unit test enforces this.
- Legacy `grafana.com/*` constants remain available (deprecated) so existing code keeps compiling;
  readers transparently fall back to them.
