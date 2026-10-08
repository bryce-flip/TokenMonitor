---
last_mapped_commit: 56e39614d8217817733b2b97293be83160ebbf2e
last_mapped_at: 2026-10-08
---
# Codebase Structure

**Analysis Date:** 2026-10-08

## Directory Layout

```text
TokenMonitor/
├── go.mod                 # Go module declaration
├── requirements.md        # MVP specification and proposed layout
└── .planning/
    └── codebase/          # Codebase analysis documents
```

No `cmd/`, `internal/`, `config/`, `migrations/`, `deployments/`, or test directories are present. Those names occur as proposals in `requirements.md`, section 22.

## Directory Purposes

**Repository root:**
- Purpose: Holds the module declaration and project requirements.
- Contains: `go.mod`, `requirements.md`.
- Key files: `go.mod`, `requirements.md`.

**`.planning/codebase/`:**
- Purpose: Holds generated codebase mapping documents.
- Contains: `ARCHITECTURE.md`, `STRUCTURE.md`, and other focus-area maps.
- Key files: `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/STRUCTURE.md`.

## Key File Locations

**Entry Points:**
- Not detected; `requirements.md`, section 22 proposes `cmd/indexer/main.go`.

**Configuration:**
- `go.mod`: Module name and Go version.
- `requirements.md`: Proposed token configuration, RPC environment variables, and confirmation settings; no runtime configuration file exists.

**Core Logic:**
- Not detected; `requirements.md`, section 22 proposes `internal/ethereum/`, `internal/indexer/`, `internal/token/`, `internal/storage/`, and `internal/metrics/`.

**Testing:**
- Not detected; no `*_test.go` files or test configuration exist.

## Naming Conventions

**Files:**
- No implementation naming convention is established by source files. Follow Go's `*_test.go` convention when tests are added.
- `requirements.md`, section 22 proposes lowercase Go filenames such as `indexer.go` and `checkpoint.go`.

**Directories:**
- No implemented package directory convention exists.
- `requirements.md`, section 22 proposes `cmd/indexer/` for the executable and domain-focused packages under `internal/`.

## Where to Add New Code

**New Feature:**
- Primary code: Start with the relevant proposed package in `requirements.md`, section 22, once that package is created; current repository has no implementation path.
- Tests: Co-locate Go `*_test.go` files with each new package; no existing test layout constrains this.

**New Component/Module:**
- Implementation: Use proposed `cmd/indexer/main.go` for the executable and `internal/` for application code only if the design in `requirements.md`, section 22 fits the feature.

**Utilities:**
- Shared helpers: No shared utility package exists. Put a helper in the package that uses it until multiple packages need it.

## Special Directories

**`.planning/codebase/`:**
- Purpose: Maps current repository state for planning.
- Generated: Yes, by codebase mapping.
- Committed: Repository policy not detected.

**Proposed `migrations/` and `deployments/`:**
- Purpose: SQL schema and Docker/Grafana deployment assets in `requirements.md`, section 22.
- Generated: Not applicable; directories do not exist.
- Committed: Not applicable in current repository.

---

*Structure analysis: 2026-10-08*
