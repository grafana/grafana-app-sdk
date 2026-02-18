---
name: go-upgrade
description: Upgrade Go and golangci-lint versions across the grafana-app-sdk project. Use when user wants to bump Go version, upgrade Go toolchain, update golangci-lint, or perform a Go version upgrade.
allowed-tools: [Bash, Read, Write, Edit, Grep, Glob]
---

# Go Version Upgrade

## Overview

This skill guides upgrading Go and/or golangci-lint versions across the grafana-app-sdk project. The project has version references scattered across go.mod files, CI workflows, Makefile, templates, and documentation.

## When to Use

Invoke this skill when:
- User wants to upgrade the Go version
- User wants to upgrade golangci-lint
- User asks to bump Go toolchain or update Go across the project

## Pre-flight

Before starting, determine:
1. **Target Go version** (e.g., `1.27.0`)
2. **Target golangci-lint version** (e.g., `v2.10.0`) -- must be built with a Go version >= target
3. Verify the new golangci-lint version supports the target Go version at https://github.com/golangci/golangci-lint/releases

## Checklist

Work through each group below. After each group, verify the change compiles or is syntactically valid.

### 1. Go module files

Update the `go` directive in every `go.mod` and `go.work`:

| File | Directive |
|------|-----------|
| `go.mod` | `go X.Y.Z` |
| `go.work` | `go X.Y.Z` |
| `logging/go.mod` | `go X.Y.Z` |
| `plugin/go.mod` | `go X.Y.Z` |
| `examples/apiserver/go.mod` | `go X.Y.Z` |

**Discovery command** (in case new modules are added):
```bash
find . -name go.mod -not -path './vendor/*'
```

After updating, run:
```bash
make update-workspace
```

### 2. Makefile

Update the linter version:

| Variable | File | Example |
|----------|------|---------|
| `LINTER_VERSION` | `Makefile` | `2.10.0` |

### 3. CI workflows

Update golangci-lint version in GitHub Actions:

| File | Field | Example |
|------|-------|---------|
| `.github/workflows/pr.yml` | `version:` in golangci-lint-action step | `v2.10.0` |

**Discovery command** (in case workflows change):
```bash
grep -rn 'golangci-lint\|LINTER_VERSION' .github/workflows/
```

### 4. Code generation templates

These templates scaffold new projects, so they should use the new Go version:

| File | What to update |
|------|----------------|
| `cmd/grafana-app-sdk/project.go` | `go X.Y` in the generated go.mod string |
| `cmd/grafana-app-sdk/templates/operator_Dockerfile.tmpl` | `FROM golang:X.Y-alpine` |

### 5. Documentation

Update version references in project docs:

| File | Lines to update |
|------|-----------------|
| `CLAUDE.md` | `make lint # Uses golangci-lint vX.Y.Z` |
| `CLAUDE.md` | `Go version: X.Y.Z (see go.mod)` |
| `CLAUDE.md` | `Primary language: Go X.Y.Z` |

### 6. Linter config

If the new golangci-lint version introduces stricter rules, fix violations:

```bash
make lint
```

Common new-version issues:
- **Deprecated comment formatting**: Add blank `//` line before `// Deprecated:` comments
- **Slice capacity hints**: Use `make([]T, 0, len(source))` instead of `make([]T, 0)`
- **Package naming conflicts**: Add exclusions in `.golangci.yml` if packages shadow stdlib names
- **New default linters**: Check release notes for newly-enabled linters

### 7. Verification

Run the full verification suite:

```bash
make lint       # Linter passes with new version
make test       # All tests pass
make build      # CLI binary builds
```

## Post-Upgrade

After all changes are made and verified:
1. Commit with message: `Upgrade to Go X.Y and golangci-lint vA.B.C`
2. PR description should list the number of linter issues fixed and categories
