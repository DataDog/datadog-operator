# CI Migration Analysis - PR #2503

## Overview

This document tracks the CI failures and fixes for the migration from `test-infra-definitions` to `datadog-agent/test/e2e-framework`.

**PR:** https://github.com/DataDog/datadog-operator/pull/2503
**Branch:** `lenaic/migrate_away_from_test-infra-definitions`

## Root Cause of CI Failures

The migration introduced a **Go workspace version conflict**:

1. **Main module** (`go.mod`) uses `k8s.io/apimachinery@v0.33.3` (required by controller-runtime)
2. **test/e2e module** (`test/e2e/go.mod`) uses `k8s.io@v0.35.0-alpha.0` (required by e2e-framework)
3. When Go workspace mode is enabled, Go unifies these to the highest version
4. This causes type incompatibilities:
   - In v0.33.3: `scheme.Validate` expects `[]string`
   - In v0.35.0: `scheme.Validate` expects `sets.Set[string]`

## Commit Analysis

### 1. `116b81a4` - Initial Migration
**Message:** "Migrate from `test-infra-definitions` to `datadog-agent/test/e2e-framework`"

**Status:** FAILED
**Error:** K8s type incompatibility + `undefined: model.NewConfig`
**Analysis:** This is the core migration commit. The CI failure is expected because workspace mode unifies K8s versions.
**Verdict:** NECESSARY - This is the actual migration

---

### 2. `bd9ffe3e` - First CI Fix Attempt
**Message:** "Fix CI: go version mismatch and workspace dependency conflicts"

**Status:** FAILED
**Error:** `undefined: model.NewConfig` (lint)
**Changes:**
- Fixed go version in test/e2e/go.mod (1.25.0 -> 1.25)
- Added `GOWORK=off` to vet, manager, managergobuild targets
- Removed sync dependency from manager target
- Added replace directive for api module

**Analysis:** Partially fixed - addressed K8s type errors but lint still failed
**Verdict:** PARTIALLY NECESSARY - The `GOWORK=off` approach is correct but incomplete

---

### 3. `18158a08` - Docker Build Fix
**Message:** "Fix CI: disable workspace mode in Docker builds"

**Status:** FAILED
**Error:** `undefined: model.NewConfig` (lint)
**Changes:**
- Added `GOWORK=off` to Dockerfiles
- Removed `go work sync` from update-golang.sh

**Analysis:** Fixed Docker builds but lint step still failed
**Verdict:** NECESSARY for Docker builds

---

### 4. `ff449d2d` - Makefile Lint/Fmt Fix
**Message:** "Fix CI: add GOWORK=off to Makefile lint and fmt targets"

**Status:** FAILED
**Error:** `pattern ./api/...: main module does not contain package`
**Changes:** Split lint and fmt targets for main module and test/e2e

**Analysis:** Introduced new error - api module not found when GOWORK=off
**Verdict:** MISTAKE - Wrong approach, api module needs replace directive

---

### 5. `05f6c7b1` - Lint API Separately
**Message:** "Fix CI: lint api module separately to fix GOWORK=off issue"

**Status:** FAILED
**Error:** `test/e2e/go.mod` needs `go mod tidy`
**Analysis:** Fixed api linting but test/e2e module issue surfaced
**Verdict:** PARTIALLY NECESSARY

---

### 6. `dc694f13` - GOWORK=off for API
**Message:** "Fix CI: add GOWORK=off for api module in lint and fmt targets"

**Status:** FAILED
**Error:** Missing go.sum entries in api module
**Analysis:** api/go.sum was incomplete for standalone builds
**Verdict:** NECESSARY but incomplete

---

### 7. `e380b94c` - Update api/go.sum
**Message:** "Fix CI: update api/go.sum with missing dependency entries"

**Status:** FAILED
**Error:** test/e2e module needs go mod tidy (same as #5)
**Analysis:** Fixed api but test/e2e resurfaced
**Verdict:** NECESSARY

---

### 8. `41c587d6` - Regenerate CRDs
**Message:** "Fix CI: regenerate CRDs, docs, and update go.mod files"

**Status:** FAILED
**Error:** Many missing go.sum entries, K8s version bumped to v0.35.0-alpha.0
**Analysis:** Made things worse - regeneration pulled in wrong K8s versions
**Verdict:** MISTAKE - Should not have regenerated with workspace mode

---

### 9. `3d72ae13` - Update main go.sum
**Message:** "Fix CI: update main module go.sum with missing dependency entries"

**Status:** FAILED
**Error:** K8s type incompatibility (sets.Set vs []string)
**Analysis:** go mod tidy upgraded K8s to v0.35.0-alpha.0
**Verdict:** MISTAKE - Wrong approach

---

### 10. `84c9dff9` - Revert K8s Dependencies
**Message:** "Fix CI: revert K8s dependencies to v0.33.3"

**Status:** FAILED
**Error:** `undefined: apicommon.HelmMigrationAnnotationKey` and other symbols
**Analysis:** Fixed K8s version but now api module symbols missing
**Verdict:** NECESSARY but revealed need for api replace directive

---

### 11. `ab5d59e5` - Add API Replace Directive
**Message:** "Fix CI: add replace directive for local api module"

**Status:** FAILED
**Error:** `verify-licenses` failed
**Analysis:** Fixed undefined symbols, new issue with licenses
**Verdict:** NECESSARY

---

### 12. `60997d3e` - Align Go Version
**Message:** "Fix CI: align Go version in test/e2e/go.mod"

**Status:** FAILED
**Error:** test/e2e trying to download datadog-operator v1.11.1
**Analysis:** test/e2e needs replace directive for local modules
**Verdict:** PARTIALLY NECESSARY

---

### 13. `57f8863f` - Update go.work.sum
**Message:** "Fix CI: update go.work.sum with missing dependency checksums"

**Status:** FAILED
**Error:** test/e2e needs go mod tidy
**Verdict:** PARTIALLY NECESSARY

---

### 14. `1a3c8751` - Run go mod tidy in update-golang.sh
**Message:** "Fix CI: run go mod tidy after setting Go version in update-golang.sh"

**Status:** FAILED
**Error:** LICENSE-3rdparty.csv outdated
**Verdict:** NECESSARY

---

### 15. `a61f217c` - Format test files
**Message:** "Fix CI: format test/e2e test files with go fmt"

**Status:** FAILED
**Error:** LICENSE-3rdparty.csv outdated
**Verdict:** NECESSARY (formatting fix)

---

### 16. `41278cdc` - Guard DDA Options
**Message:** "Fix: guard DDA options when operator is disabled in Kind-VM path"

**Status:** FAILED
**Error:** LICENSE-3rdparty.csv outdated
**Analysis:** Functional fix from code review, not CI fix
**Verdict:** NECESSARY (code fix)

---

### 17. `3e1d3371` - Revert Unnecessary CI Fixes
**Message:** "Revert unnecessary CI fixes: keep only migration changes"

**Status:** FAILED
**Error:** K8s type incompatibility + undefined model.NewConfig
**Analysis:** Reverted too much - broke the build again
**Verdict:** MISTAKE - Should not have reverted go.mod changes

---

### 18. `2e135beb` - Apply Code Review Suggestions
**Message:** "Apply suggestions from code review"

**Status:** FAILED
**Error:** K8s type incompatibility
**Analysis:** Code review changes, duplicate import issue
**Verdict:** NECESSARY (code review)

---

### 19. `6d79dc7a` - Fix Duplicate Import
**Message:** "Apply suggestion from @L3n41c"

**Status:** FAILED
**Error:** K8s type incompatibility
**Analysis:** Fixed duplicate import from previous commit
**Verdict:** NECESSARY

---

### 20. `cab9cb12` - Disable Workspace Mode
**Message:** "Fix build: disable Go workspace mode to avoid K8s version conflicts"

**Status:** FAILED
**Error:** K8s type incompatibility (vet target missing GOWORK=off)
**Analysis:** Added GOWORK=off to builds but missed vet target
**Verdict:** NECESSARY but incomplete

---

### 21. `7645f444` - Add API Replace Directive
**Message:** "Fix build: add api replace directive and isolate module builds"

**Status:** FAILED
**Error:** Missing go.sum entries after go work sync
**Analysis:** go work sync modified versions causing go.sum mismatch
**Verdict:** NECESSARY but go work sync was problematic

---

### 22. `44e76665` - Remove Sync from Manager
**Message:** "Fix build: remove sync from manager dependencies"

**Status:** FAILED
**Error:** LICENSE-3rdparty.csv outdated
**Analysis:** Fixed go.sum issue, license check remains
**Verdict:** NECESSARY

---

### 23. `a3039b30` - Align Go Version Format
**Message:** "Fix check-golang-version: align Go version in test/e2e/go.mod"

**Status:** FAILED
**Error:** test/e2e can't find local modules + license issue
**Verdict:** NECESSARY but incomplete

---

### 24. `6740d461` - Add Replace Directives and Avoid go work sync
**Message:** "Fix CI: add replace directives and avoid go work sync"

**Status:** FAILED (pending verification)
**Error:** LICENSE-3rdparty.csv outdated
**Changes:**
- Added replace directives to test/e2e/go.mod
- Replaced `go work sync` with individual `go mod tidy` calls

**Verdict:** NECESSARY - Core fix for module resolution

---

## Summary of Necessary Changes

### Strictly Necessary Changes

1. **test/e2e/go.mod replace directives:**
   ```go
   replace (
       github.com/DataDog/datadog-operator => ../..
       github.com/DataDog/datadog-operator/api => ../../api
   )
   ```

2. **go.mod replace directive for api:**
   ```go
   replace github.com/DataDog/datadog-operator/api => ./api
   ```

3. **GOWORK=off in Makefile targets** for: vet, lint, fmt, test, manager, etc.

4. **GOWORK=off in Dockerfiles** for all go build commands

5. **hack/update-golang.sh:** Replace `go work sync` with individual `go mod tidy` calls with GOWORK=off

6. **Code fixes from code review:**
   - Guard DDA options when operator disabled
   - Import formatting

### Unnecessary/Mistake Changes

1. **CRD/docs regeneration** (commit 41c587d6) - Pulled in wrong K8s versions
2. **Revert commit** (3e1d3371) - Broke the build by reverting necessary go.mod changes
3. **Multiple go.sum updates** that got reverted or caused version conflicts

---

### 25. `3841b450` - Update LICENSE-3rdparty.csv
**Message:** "Fix CI: update LICENSE-3rdparty.csv with dependency changes"

**Status:** PARTIAL SUCCESS - GitHub Actions passed, GitLab CI failed
**Changes:**
- Removed obsolete dependencies (viper, hcl, mapstructure, etc.)
- Added new dependency (aws-sdk-go-v2/service/signin)
- Updated license for cyphar/filepath-securejoin (BSD-3-Clause -> MPL-2.0)
- Added sigs.k8s.io/structured-merge-diff/v6

**Analysis:** License file was outdated due to dependency changes from the migration. GitHub Actions passed but GitLab CI `check-golang-version` still failing.
**Verdict:** NECESSARY but incomplete

---

### 26. `51e073a9` - Update CI_MIGRATION_ANALYSIS.md
**Message:** "Update CI migration analysis: all CI checks now passing"

**Status:** FAILED - GitLab CI `check-golang-version` still failing
**Changes:**
- Updated CI_MIGRATION_ANALYSIS.md with status

**Analysis:** Status update commit. The `dd-gitlab/check-golang-version` was still failing.
**Verdict:** DOCUMENTATION ONLY

---

### 27. `a0364a51` - Fix Go version format and go.sum files
**Message:** "Fix CI: align Go version format and update go.sum files"

**Status:** ✅ SUCCESS - All CI checks passed
**Changes:**
- Changed `test/e2e/go.mod` go version from `1.25.0` to `1.25` (format consistency)
- Added 16 missing checksum entries to `api/go.sum`
- Added 11 missing checksum entries to `go.work.sum`

**Analysis:** The `check-golang-version` CI check runs `make update-golang` and verifies no diff. The individual `go mod tidy` calls in `update-golang.sh` were producing changes because:
1. `test/e2e/go.mod` had `go 1.25.0` instead of `go 1.25`
2. go.sum files were missing checksums that `go mod tidy` adds

**Verdict:** NECESSARY - Fixed the `check-golang-version` failure

---

## Current Status

**Last commit:** `5ca792c3`
**CI Status:** ✅ ALL CHECKS PASSED

GitHub Actions checks:
- CodeQL: ✅ (skipping - no relevant changes)
- build (validation): ✅
- build (pull request linter): ✅
- build-linux-binary: ✅
- build-darwin-binary: ✅
- build-windows-binary: ✅
- Check Milestone: ✅
- Analyze (go): ✅
- Analyze (python): ✅
- DDCI Task Sourcing: ✅

GitLab CI checks:
- dd-gitlab/check-golang-version: ✅
- dd-gitlab/build: ✅
- devflow/mergegate: ✅

## Lessons Learned

1. **Never run `go work sync`** in CI or scripts - it unifies versions across modules
2. **Always use `GOWORK=off`** when building individual modules in a workspace
3. **Each module needs replace directives** to reference local modules when GOWORK=off
4. **Don't regenerate CRDs/docs** when dependency versions are in flux
5. **Fix one issue at a time** and verify CI before moving to next issue
6. **Go version format matters**: `go 1.25.0` is different from `go 1.25` in go.mod files
7. **go.sum files must be synchronized**: When running `go mod tidy` with GOWORK=off, each module's go.sum must have all required checksums

## Validation Checklist

All checks passed:
- [x] `GOWORK=off go vet ./...` passes in root
- [x] `cd api && GOWORK=off go vet ./...` passes
- [x] `cd test/e2e && GOWORK=off go fmt ./...` passes
- [x] `make verify-licenses` passes
- [x] `make check-golang-version` passes (no git diff)
- [x] Docker image builds pass

## Migration Complete

The migration from `test-infra-definitions` to `datadog-agent/test/e2e-framework` is now complete with all CI checks passing.
