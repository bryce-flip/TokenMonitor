---
last_mapped_commit: 75f060c579368cd601a8d60ea40e6790a8509ec5
last_mapped_at: 2026-10-08
---
# Testing Patterns

**Analysis Date:** 2026-10-08

## Test Framework

**Runner:**
- Not detected; `go.mod` declares Go 1.27.1, but the repository has no `*_test.go` files or test configuration.
- Config: Not detected (`go.mod`).

**Assertion Library:**
- Not detected; `go.mod` has no dependencies.

**Run Commands:**

```bash
go test ./...          # Standard Go command once packages exist; no project test script exists
```

## Test File Organization

**Location:**
- Not established; no tests or source packages exist (`go.mod`).

**Naming:**
- Not established locally. Use Go's `*_test.go` convention when tests are added under the package they exercise; `requirements.md` does not prescribe a test layout.

**Structure:**

```text
go.mod                # Module declaration; no test directories or files yet
requirements.md       # Acceptance criteria and validation requirements
```

## Test Structure

**Suite Organization:**

```go
// Not detected: no test suite exists in this repository.
```

**Patterns:**
- Setup pattern: Not detected (`go.mod`).
- Teardown pattern: Not detected (`go.mod`).
- Assertion pattern: Not detected (`go.mod`).

## Mocking

**Framework:** Not detected (`go.mod`).

**Patterns:**

```go
// Not detected: no mocks exist in this repository.
```

**What to Mock:**
- No established practice. `requirements.md` specifies Ethereum RPC and ClickHouse interactions; tests should isolate external calls when implementing those boundaries.

**What NOT to Mock:**
- No established practice. Keep deterministic mint/burn classification and metric calculations directly testable as specified by `requirements.md`.

## Fixtures and Factories

**Test Data:**

```go
// Not detected: no fixtures or factories exist in this repository.
```

**Location:**
- Not established (`go.mod`).

## Coverage

**Requirements:** No percentage or coverage gate is defined in `requirements.md`.

**View Coverage:**

```bash
go test -cover ./...    # Standard Go command once packages exist
```

## Test Types

**Unit Tests:**
- Not implemented. `requirements.md` defines behavior suitable for focused tests: mint/burn identification, supply calculations, and issuance metrics.

**Integration Tests:**
- Not implemented. `requirements.md` defines RPC ingestion, ClickHouse storage, restart recovery, and contract-supply checks as acceptance criteria.

**E2E Tests:**
- Not used; no E2E framework or runnable application exists (`go.mod`).

## Common Patterns

**Async Testing:**

```go
// Not detected: no concurrent or asynchronous tests exist.
```

**Error Testing:**

```go
// Not detected: no error-path tests exist.
```

---

*Testing analysis: 2026-10-08*
