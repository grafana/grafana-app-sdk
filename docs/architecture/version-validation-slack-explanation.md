# Version-Specific Validation Problem Scenario

## The Problem

When we change validation rules in the backend, frontend applications that have hardcoded validation logic can become out of sync. This creates a mismatch where frontend and backend enforce different rules.

## Example Scenario: Branch Name Validation

**Initial Backend Rules**: Branch names must match pattern `^[a-z0-9-]+$` (only lowercase letters, numbers, and hyphens)
- Users can enter: `feature-branch`, `main`, `v1-0-0`
- Users cannot enter: `feature.user.login`, `user_login` (dots and underscores not allowed)

**Backend Changes**: We update the backend to allow dots and underscores in branch names → pattern becomes `^[a-z0-9._-]+$`
- Users can now enter: `feature.user.login`, `user_login`, `feature-branch`
- The text/format allowed in the input field changes

## What Happens

**Scenario 1: Backend changed, frontend not updated**
- Backend now allows `feature.user.login` (dots allowed) - input field text format changed
- Frontend still has old validation rules hardcoded (rejects `feature.user.login`)
- **Result**: User enters valid text in input field, but frontend rejects it before submission → user confusion
- **User sees**: Frontend error message like "Branch name can only contain lowercase letters, numbers, and hyphens" (outdated message)
- **What should happen**: Backend accepts `feature.user.login` and resource is created successfully

**Scenario 2: Frontend accepts, backend rejects**
- Frontend doesn't validate dots, accepts `feature.user.login` (user enters text in input field)
- Backend still enforces old rules, rejects `feature.user.login` (dots not allowed)
- **Result**: User enters text in input field, submits form, backend rejects it → confusion
- **User sees**: HTTP 422 error with message like "Repository 'my-repo' is invalid" and details:
  ```json
  {
    "details": {
      "causes": [
        {
          "type": "FieldValueInvalid",
          "field": "spec.github.branch",
          "message": "branch name must match pattern ^[a-z0-9-]+$ for API version v1alpha1"
        }
      ]
    }
  }
  ```
- **User experience**: User entered text, clicked submit, got error after submission → poor UX

## The Core Issue

When we change validation rules in the backend:
- The text/format allowed in input fields changes
- Frontend with hardcoded validation becomes out of sync
- Users get confusing errors when frontend and backend don't match
- Users may enter text that frontend accepts but backend rejects (or vice versa)
- Duplicating validation logic in frontend creates maintenance burden
- Frontend must be updated every time we change backend validation rules

## Translation/Details Challenge

The problem is about:
- **Details**: Which validation rules does the backend currently enforce?
- **Translation**: How do we ensure frontend validation matches backend validation when backend rules change?
- **Error Messages**: What error text should be displayed to users? Should it mention the API version? Should it show the pattern?
- **fieldError Details**: When backend returns validation errors, they include:
  ```json
  {
    "type": "FieldValueInvalid",
    "field": "spec.github.branch",
    "detail": "branch name must match pattern ^[a-z0-9-]+$ for API version v1alpha1"
  }
  ```
  - **Type**: Machine-readable error type (FieldValueInvalid, FieldValueRequired, etc.)
  - **Field**: JSON path to the field (e.g., `spec.github.branch`)
  - **Detail**: Human-readable error message that should guide the user
  - **Origin**: (Optional) Where the error originated (validator name, service, etc.)

## The Solution

Use version-specific APIs so that:
- Different API versions can have different validation rules
- Backend validates based on the API version in the request
- Frontend doesn't need to duplicate validation logic - it relies on backend validation
- When backend changes rules, we introduce a new API version rather than changing existing ones
