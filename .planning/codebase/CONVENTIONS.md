---
last_mapped_commit: 56e39614d8217817733b2b97293be83160ebbf2e
last_mapped_at: 2026-10-08
---
# Coding Conventions

**Analysis Date:** 2026-10-08

## Naming Patterns

**Files:**
- No source-file naming pattern is established. The repository contains `go.mod` and `requirements.md`; the proposed `.go` layout in `requirements.md` is a specification, not implemented convention.

**Functions:**
- Not detected; there are no Go functions in the repository (`go.mod`).

**Variables:**
- Not detected; there are no Go variables in the repository (`go.mod`).

**Types:**
- Not detected; there are no Go type declarations in the repository (`go.mod`).

## Code Style

**Formatting:**
- Not detected; no source or formatter configuration exists alongside `go.mod`.
- Follow standard `gofmt` formatting when adding the first Go files; no project-specific settings are defined in `requirements.md`.

**Linting:**
- Not detected; there is no linter configuration or lint command in `go.mod` or `requirements.md`.

## Import Organization

**Order:**
1. Not established; no Go import declarations exist (`go.mod`).

**Path Aliases:**
- Not detected. The module path is `TOkenMonitor` in `go.mod`.

## Error Handling

**Patterns:**
- No implementation pattern exists (`go.mod`). `requirements.md` requires RPC error retry, rate-limit handling, and clear logs for RPC errors, stalled indexing, and supply mismatches.

## Logging

**Framework:** Not detected (`go.mod`).

**Patterns:**
- No logging implementation exists. `requirements.md` calls for explicit RPC, indexer-stall, and supply-mismatch logs; it does not select a logging library.

## Comments

**When to Comment:**
- Not established in source (`go.mod`). `requirements.md` documents domain behavior, but contains no implemented comment conventions.

**JSDoc/TSDoc:**
- Not applicable to the Go module in `go.mod`.

## Function Design

**Size:** Not established; no functions exist (`go.mod`).

**Parameters:** Not established; no functions exist (`go.mod`).

**Return Values:** Not established; no functions exist (`go.mod`).

## Module Design

**Exports:** Not established; `go.mod` declares a module but no packages exist.

**Barrel Files:** Not applicable; no source files exist (`go.mod`).

---

*Convention analysis: 2026-10-08*
