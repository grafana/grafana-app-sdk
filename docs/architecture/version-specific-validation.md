# Version-Specific Validation: Frontend and Backend Mismatches

## Overview

When API versions evolve, validation rules may change between versions. This document explains how to handle validation when frontend and backend have different rules, and how `fieldErrors` in status helps surface these mismatches.

## Problem Scenario

Consider a scenario where branch name validation rules change between API versions:

- **v1alpha1**: Branch names must match pattern `^[a-z0-9-]+$` (lowercase letters, numbers, and hyphens only)
- **v1beta1**: Branch names must match pattern `^[a-z0-9._-]+$` (allows dots and underscores)

### The Mismatch Problem

**Scenario**: Backend is upgraded to support `v1beta1`, but frontend is still using `v1alpha1` validation rules.

1. **Frontend** (using `v1alpha1` rules): Rejects branch name `feature/user.login` because it contains dots
2. **Backend** (using `v1beta1` rules): Accepts branch name `feature/user.login` because dots are allowed
3. **Result**: User cannot create resources with valid branch names because frontend validation is too strict

Or the reverse:

1. **Frontend** (using `v1alpha1` rules): Accepts branch name `feature/user.login` (doesn't validate dots)
2. **Backend** (using `v1alpha1` rules): Rejects branch name `feature/user.login` because dots are not allowed
3. **Result**: User submits form, but backend rejects it, causing confusion

## Solution: Use `fieldErrors` in Status

Instead of duplicating validation logic in the frontend, rely on backend validation and `fieldErrors` in status to surface validation errors.

> **Note**: For complete information on `fieldErrors` in status, see [Field Errors in Status](./field-errors-in-status.md).

### Backend Implementation

**Admission Validator** (format validation - version-specific):

```go
// Admission validator enforces format rules per API version
func (v *AdmissionValidator) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
    obj := a.GetObject()
    repo := obj.(*provisioning.Repository)
    
    // Get the API version from the request
    gvk := a.GetKind()
    apiVersion := gvk.Version
    
    // Version-specific format validation
    var branchPattern *regexp.Regexp
    switch apiVersion {
    case "v1alpha1":
        // v1alpha1: only lowercase letters, numbers, and hyphens
        branchPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
    case "v1beta1":
        // v1beta1: allows dots and underscores
        branchPattern = regexp.MustCompile(`^[a-z0-9._-]+$`)
    default:
        return apierrors.NewBadRequest(fmt.Sprintf("unsupported API version: %s", apiVersion))
    }
    
    // Validate branch name format
    if !branchPattern.MatchString(repo.Spec.GitHub.Branch) {
        return apierrors.NewInvalid(
            provisioning.RepositoryGroupVersionKind.GroupKind(),
            repo.Name,
            field.ErrorList{
                field.Invalid(
                    field.NewPath("spec", "github", "branch"),
                    repo.Spec.GitHub.Branch,
                    fmt.Sprintf("branch name must match pattern %s for API version %s", branchPattern.String(), apiVersion),
                ),
            },
        )
    }
    
    return nil
}
```

**Controller** (runtime validation - version-agnostic):

```go
// Controller performs runtime validation (version-agnostic)
func (c *RepositoryController) reconcile(ctx context.Context, repo *provisioning.Repository) error {
    var fieldErrors []ErrorDetails
    
    // Runtime validation: check if branch exists (same for all versions)
    branchRef := fmt.Sprintf("refs/heads/%s", repo.Spec.GitHub.Branch)
    _, err := c.gitClient.GetRef(ctx, branchRef)
    if err != nil {
        if errors.Is(err, nanogit.ErrObjectNotFound) {
            fieldErrors = append(fieldErrors, ErrorDetails{
                Type:   metav1.CauseTypeFieldValueInvalid,
                Field:  "spec.github.branch",
                Detail: "branch not found", // Actionable: user should use an existing branch
            })
        } else {
            fieldErrors = append(fieldErrors, ErrorDetails{
                Type:   metav1.CauseTypeFieldValueInvalid,
                Field:  "spec.github.branch",
                Detail: fmt.Sprintf("failed to check if branch exists: %v", err),
            })
        }
    }
    
    // Update fieldErrors in status
    patchOps := []map[string]interface{}{
        {
            "op":    "replace",
            "path":  "/status/fieldErrors",
            "value": fieldErrors,
        },
    }
    
    return c.statusPatcher.Patch(ctx, repo, patchOps...)
}
```

### Frontend Implementation

**Don't duplicate validation logic in frontend** - let the backend be the source of truth:

```typescript
// ❌ Wrong: Duplicating validation logic in frontend
function validateBranchName(branch: string, apiVersion: string): boolean {
  if (apiVersion === 'v1alpha1') {
    return /^[a-z0-9-]+$/.test(branch);
  } else if (apiVersion === 'v1beta1') {
    return /^[a-z0-9._-]+$/.test(branch);
  }
  return false;
}

// ✅ Correct: Use backend validation and fieldErrors
async function createRepository(spec: RepositorySpec) {
  try {
    // Step 1: Validate with dryRun (backend validates format)
    await api.createResource(spec, { dryRun: true });
    
    // Step 2: Create resource
    const resource = await api.createResource(spec);
    
    // Step 3: Wait for reconciliation and check fieldErrors
    const reconciled = await waitForReconciliation(resource.metadata.name);
    
    // Step 4: Display any runtime errors from fieldErrors
    if (reconciled.status.fieldErrors?.length > 0) {
      reconciled.status.fieldErrors.forEach(error => {
        if (error.field === 'spec.github.branch') {
          setFieldError('branch', error.detail);
        }
      });
    }
  } catch (error) {
    // Handle admission errors (format validation)
    if (error.status === 422 && error.body?.details?.causes) {
      error.body.details.causes.forEach((cause: any) => {
        if (cause.field === 'spec.github.branch') {
          setFieldError('branch', cause.message);
        }
      });
    } else {
      throw error;
    }
  }
}
```

## Example: Version Mismatch Scenario

### Scenario: Backend Upgraded, Frontend Not Updated

**Backend**: Supports `v1beta1` (allows dots and underscores in branch names)
**Frontend**: Still using `v1alpha1` client (only allows hyphens)

**User Action**: User tries to create repository with branch `feature/user.login`

**What Happens**:

1. **Frontend** (if it had validation): Would reject `feature/user.login` ❌
2. **Backend** (admission): Accepts `feature/user.login` ✅ (valid for `v1beta1`)
3. **Backend** (controller): Checks if branch exists, populates `fieldErrors` if not found

**With `fieldErrors` approach**:

```typescript
// Frontend doesn't validate format - backend does
const repo = await api.createResource({
  apiVersion: 'provisioning.grafana.app/v1beta1',
  kind: 'Repository',
  spec: {
    github: {
      branch: 'feature/user.login' // Frontend doesn't validate this
    }
  }
});

// Backend admission validates format (v1beta1 allows dots)
// If format is invalid, returns HTTP 422 with error details

// If format is valid, resource is created and controller checks if branch exists
const reconciled = await waitForReconciliation(repo.metadata.name);

// Display runtime errors from fieldErrors
if (reconciled.status.fieldErrors?.length > 0) {
  // Show: "branch not found" if branch doesn't exist
  // Frontend doesn't need to know version-specific format rules
}
```

### Benefits

1. **Single Source of Truth**: Backend defines validation rules, frontend doesn't duplicate them
2. **Version Independence**: Frontend doesn't need to know version-specific rules
3. **Automatic Updates**: When backend adds new version, frontend automatically gets correct validation
4. **Clear Errors**: Users see actionable errors from backend, not confusing frontend validation

## Best Practices

### 1. Format Validation in Admission (Version-Specific)

- **Where**: Admission webhook
- **When**: Before resource is persisted
- **What**: Format, structure, type checks (version-specific rules)
- **Error Format**: HTTP 422 with `details.causes`

```go
// Admission validator handles version-specific format rules
func (v *AdmissionValidator) Validate(...) error {
    // Check API version
    // Apply version-specific format validation
    // Return admission errors if invalid
}
```

### 2. Runtime Validation in Controller (Version-Agnostic)

- **Where**: Controller reconciliation
- **When**: After resource is persisted
- **What**: External system checks, dynamic state validation
- **Error Format**: `fieldErrors` in status

```go
// Controller handles runtime validation (same for all versions)
func (c *Controller) reconcile(...) error {
    // Check external systems (branch exists, repo exists, etc.)
    // Populate fieldErrors in status
}
```

### 3. Frontend: Don't Duplicate Validation

- **Don't**: Implement format validation in frontend
- **Do**: Use `dryRun=true` for pre-creation validation
- **Do**: Read `fieldErrors` from status for runtime errors
- **Do**: Display errors from backend (admission or status)

```typescript
// ✅ Correct: Use backend validation
try {
  await api.createResource(spec, { dryRun: true });
  const resource = await api.createResource(spec);
  const reconciled = await waitForReconciliation(resource.metadata.name);
  // Display errors from reconciled.status.fieldErrors
} catch (error) {
  // Display errors from error.body.details.causes (admission errors)
}
```

## Migration Strategy

When adding a new API version with different validation rules:

1. **Backend**: Implement version-specific validation in admission webhook
2. **Backend**: Keep runtime validation version-agnostic in controller
3. **Frontend**: Update API client to support new version (no validation logic changes needed)
4. **Frontend**: Use `dryRun=true` and `fieldErrors` to surface validation errors
5. **Testing**: Test with mismatched frontend/backend versions to ensure errors are surfaced correctly

## Summary

- **Format validation** (version-specific): Handle in admission webhook, return HTTP 422 errors
- **Runtime validation** (version-agnostic): Handle in controller, populate `fieldErrors` in status
- **Frontend**: Don't duplicate validation logic - use backend validation via `dryRun` and `fieldErrors`
- **Benefits**: Single source of truth, version independence, automatic updates, clear error messages

## Related Documentation

- [Field Errors in Status](./field-errors-in-status.md) - Complete guide on using `fieldErrors` for runtime validation
- [Admission Control](../admission-control.md) - How to implement admission webhooks for format validation
